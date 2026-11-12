package device

import (
	"fmt"
	"time"
)

func (j *restoreJob) espressifCandidates() []Candidate {
	all, err := j.candidates()
	if err != nil {
		return nil
	}
	return espressif(all)
}

// autoConnect waits for exactly one Espressif USB device and catches it in ROM download
// mode. A port that appears for only a few hundred milliseconds is still caught.
func (j *restoreJob) autoConnect() (Device, error) {
	j.stage("waiting")
	j.log("Scanning for an Espressif USB device; a port that appears for only a few hundred ms is still caught")
	start := time.Now()
	deadline := start.Add(j.timeout)
	var lastNotice, lastCapture, nextHint time.Time
	present, hintSent, multiSent := false, false, false
	var hints []string
	lastError := ""

	probe := func() (Device, error) {
		romDeadline := time.Now().Add(romProbe)
		if romDeadline.After(deadline) {
			romDeadline = deadline
		}
		for time.Now().Before(romDeadline) {
			if err := j.checkCancel(); err != nil {
				return nil, err
			}
			found := j.espressifCandidates()
			if len(found) == 1 {
				d, err := j.connect(found[0].Device, "no-reset", 10, j.raw)
				if err != nil {
					lastError = "ROM handshake: " + err.Error()
					return nil, nil
				}
				j.port = found[0].Device
				j.stage("connecting")
				j.log("Caught %s (%s)", found[0].Device, found[0].Description)
				return d, nil
			}
			time.Sleep(j.interval)
		}
		return nil, nil
	}
	tryCapture := func(c Candidate) (Device, error) {
		if !j.capture(c) {
			return nil, nil
		}
		return probe()
	}

	for time.Now().Before(deadline) {
		if err := j.checkCancel(); err != nil {
			return nil, err
		}
		now := time.Now()
		if !now.Before(nextHint) {
			hints = j.hints()
			nextHint = now.Add(time.Second)
			if len(hints) == 1 && !hintSent {
				hintSent = true
				j.emit(Event{Type: "hint", Code: "code43", Port: hints[0]})
				j.log("USB Code 43 seen; watching the last known port %s. Unplug and replug the device.", hints[0])
			}
		}
		found := j.espressifCandidates()
		switch {
		case len(found) > 1:
			if !multiSent || now.Sub(lastNotice) >= 5*time.Second {
				j.emit(Event{Type: "hint", Code: "multiple"})
				j.log("More than one Espressif device is connected; keep only the one to restore")
				multiSent, lastNotice = true, now
			}
			time.Sleep(j.interval)
			continue
		case len(found) == 1:
			if !present || now.Sub(lastCapture) >= 3*time.Second {
				lastCapture = now
				d, err := tryCapture(found[0])
				if d != nil || err != nil {
					return d, err
				}
			}
			present = true
		default:
			present = false
			if len(hints) == 1 {
				d, err := tryCapture(Candidate{Device: hints[0], Description: "Espressif USB (Code 43)", VID: EspressifVID, PID: USBJTAGPID})
				if d != nil || err != nil {
					return d, err
				}
			}
		}
		if now.Sub(lastNotice) >= 10*time.Second {
			j.emit(Event{Type: "hint", Code: "replug"})
			msg := "Still waiting for the device"
			if lastError != "" {
				msg += " (last error: " + lastError + ")"
			}
			j.log("%s", msg)
			lastNotice = now
		}
		time.Sleep(j.interval)
	}
	msg := fmt.Sprintf("No device was caught within %d s. Close any serial monitor, then hold BOOT while plugging in USB.", int(j.timeout.Seconds()))
	if lastError != "" {
		msg += " Last error: " + lastError
	}
	return nil, coded("not_found", "%s", msg)
}

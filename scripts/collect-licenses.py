"""Preserve license files from the exact bundled Python distributions."""
from importlib.metadata import distributions
from pathlib import Path
import shutil
import sys
out=Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
for dist in distributions():
    for item in dist.files or []:
        if any(word in item.name.lower() for word in ('license','copying','notice','copyright')):
            source=Path(dist.locate_file(item))
            if source.is_file():
                dest=out/(dist.metadata['Name']+'-'+dist.version)/str(item)
                dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(source,dest)

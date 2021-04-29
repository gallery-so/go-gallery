FROM ubuntu:20.04





RUN useradd -b /home/gly




ADD bin/main /home/gly/main


ENTRYPOINT ["/home/gly/main"]
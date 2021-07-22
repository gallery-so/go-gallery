FROM ubuntu:20.10





RUN useradd -m glry

RUN apt-get update
RUN apt-get install -y ca-certificates wget

WORKDIR /home/glry
ADD bin/.env .env
ADD bin/main main

CMD "./main"
# ENTRYPOINT ["/home/glry/main"]
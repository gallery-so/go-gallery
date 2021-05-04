FROM ubuntu:20.04





RUN useradd -m glry


WORKDIR /home/glry
ADD bin/.env .env
ADD bin/main main

CMD "./main"
# ENTRYPOINT ["/home/glry/main"]
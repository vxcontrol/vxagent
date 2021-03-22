# Dockerfile for publishing build to repo
FROM debian:buster-slim

RUN mkdir -p /opt/vxagent/bin
RUN mkdir -p /opt/vxagent/data
RUN mkdir -p /opt/vxagent/logs

ADD preparing.sh /opt/vxagent/bin/
ADD build/vxagent /opt/vxagent/bin/

WORKDIR /opt/vxagent

RUN chmod +x /opt/vxagent/bin/preparing.sh
RUN /opt/vxagent/bin/preparing.sh

RUN apt update
RUN apt install -y ca-certificates
RUN apt clean

ENTRYPOINT ["/opt/vxagent/bin/vxagent"]

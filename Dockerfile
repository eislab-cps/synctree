FROM alpine

WORKDIR /
COPY ./bin/helloworld /bin

CMD ["helloworld", "talk"]

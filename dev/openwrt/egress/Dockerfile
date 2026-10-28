FROM alpine:3.20
RUN apk add --no-cache python3 iproute2 curl
COPY peer.py /lab/peer.py
ENTRYPOINT ["python3", "/lab/peer.py"]

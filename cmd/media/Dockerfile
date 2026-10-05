# The image has no shell, so the blob directory is made here. A new volume mounted over it takes its
# ownership, which lets the nonroot user write.
FROM busybox:1.37 AS dirs
RUN mkdir -p /media && chown 65532:65532 /media

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=dirs --chown=65532:65532 /media /home/nonroot/media

ARG TARGETOS
ARG TARGETARCH
COPY bin/media_${TARGETOS}_${TARGETARCH} /service

ENTRYPOINT ["/service"]

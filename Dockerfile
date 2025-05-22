FROM scratch
VOLUME /data

COPY octoplex /octoplex

ENV OCTO_DOCKER="true"
ENV OCTO_DATA_DIR="/data"

ENTRYPOINT ["/octoplex"]
CMD ["server", "start"]

FROM scratch
COPY octoplex ./octoplex

ENV OCTO_DOCKER="true"

ENTRYPOINT ["/octoplex", "server"]
CMD ["start"]

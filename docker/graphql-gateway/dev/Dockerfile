FROM node:18-bullseye

WORKDIR /workspace

RUN curl -sSL https://rover.apollo.local/nix/v0.11.1 | sh
RUN curl -sSL https://router.apollo.local/download/nix/latest | sh

ADD graphql/schema/schema.graphql .
ADD docker/graphql-gateway/dev/supergraph.yaml .
ADD docker/graphql-gateway/dev/router.yaml .

ENV APOLLO_ELV2_LICENSE=accept
RUN ~/.rover/bin/rover supergraph compose --config ./supergraph.yaml > ./supergraph-schema.graphql

CMD ./router --dev --config ./router.yaml --supergraph ./supergraph-schema.graphql
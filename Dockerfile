FROM alpine:3.9

ENV MONGODB_TOOLS_VERSION 4.0.5-r0
ENV MONGODB_VERSION 4.0.5-r0
ENV GOOGLE_CLOUD_SDK_VERSION 232.0.0
ENV AZURE_CLI_VERSION 2.0.76
ENV KUBE_LATEST_VERSION v1.10.0
ENV PATH /root/google-cloud-sdk/bin:$PATH

LABEL org.label-schema.name="kube-backup" \
      org.label-schema.auth="Harish Anchu <harishanchu@wimoku.com>" \
      org.label-schema.description="Kuberentes backup automation tool" \
      org.label-schema.url="https://gitlab.4medica.net/gke/kube-backup/kube-backup" \
      org.label-schema.vcs-url="https://gitlab.4medica.net/gke/kube-backup/kube-backup" \
      org.label-schema.vendor="4medica.net"

RUN apk add --no-cache ca-certificates mongodb-tools=${MONGODB_TOOLS_VERSION}
RUN apk add --no-cache ca-certificates mongodb=${MONGODB_VERSION}
RUN wget https://dl.minio.io/client/mc/release/linux-amd64/mc -P /usr/bin
RUN chmod u+x /usr/bin/mc

WORKDIR /root/

#install gcloud
# https://github.com/GoogleCloudPlatform/cloud-sdk-docker/blob/69b7b0031d877600a9146c1111e43bc66b536de7/alpine/Dockerfile
RUN apk --no-cache add \
        curl \
        python \
        py-crcmod \
        bash \
        libc6-compat \
        openssh-client \
        git \
    && curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${GOOGLE_CLOUD_SDK_VERSION}-linux-x86_64.tar.gz && \
    tar xzf google-cloud-sdk-${GOOGLE_CLOUD_SDK_VERSION}-linux-x86_64.tar.gz && \
    rm google-cloud-sdk-${GOOGLE_CLOUD_SDK_VERSION}-linux-x86_64.tar.gz && \
    ln -s /lib /lib64 && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud config set metrics/environment github_docker_image && \
    gcloud --version

# install azure-cli
#RUN apk add py-pip && \
#  pip install --upgrade pip && \
#  apk add --virtual=build gcc libffi-dev musl-dev openssl-dev python-dev make && \
#  pip install azure-cli==${AZURE_CLI_VERSION} && \
#  apk del --purge build

# install kubectl
RUN apk add --update ca-certificates \
 && apk add --update -t deps curl \
 && apk add --update gettext \
 && curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl \
 && chmod +x /usr/local/bin/kubectl \
 && apk del --purge deps \
 && rm /var/cache/apk/*

COPY kube-backup /bin

VOLUME ["/kube-backup"]

ENTRYPOINT [ "kube-backup" ]

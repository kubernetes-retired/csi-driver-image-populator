FROM centos:centos7
LABEL maintainers="Kubernetes Authors"
LABEL description="Image Driver"

COPY ./bin/imagepopulatorplugin /imagepopulatorplugin
COPY ./bin/cp-static /cp-static
ENTRYPOINT ["/imagepopulatorplugin"]


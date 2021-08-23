# csi-driver-image-populator

CSI driver that uses a container image as a volume.

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing List](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

## How it works:

Currently the driver makes use of buildah to download the container image if it is not already available, launch a new instance of it named after the volumeHandle, and mount it.

In the future, integration with CRI would be desirable so the driver could ask via CRI that the Container Runtime perform these activities in a generic way.

## Usage:

**This is a prototype driver. Do not use for production**

It also requires features that are still in development.

### Build imageplugin
```
$ make image
```

### Installing into Kubernetes
```
deploy/deploy-image.sh
```

### Example Usage in Kubernetes
```
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: nginx
    image: nginx:1.13-alpine
    ports:
    - containerPort: 80
    volumeMount:
    - name: data
      mountPath: /usr/share/nginx/html
  volumes:
  - name: data
    csi:
      driver: image.csi.k8s.io
      volumeAttributes:
          image: kfox1111/misc:test
```

### Start Image driver manually
```
$ sudo ./bin/imageplugin --endpoint tcp://127.0.0.1:10000 --nodeid CSINode -v=5
```

### Mount the image
$ csc -e tcp://127.0.0.1:10000 node publish abcdefg --vol-context image=kfox1111/misc:test --cap MULTI_NODE_MULTI_WRITER,block --target-path /tmp/csi

### Unmount the image
$ csc -e tcp://127.0.0.1:10000 node unpublish abcdefg --target-path /tmp/csi

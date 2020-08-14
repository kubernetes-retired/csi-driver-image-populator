/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package image

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	. "github.com/otiai10/copy"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	deviceID         = "deviceID"
	cpStaticLocation = "/cp-static"
)

var (
	TimeoutError = fmt.Errorf("Timeout")
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	criConn  *grpc.ClientConn
	Timeout  time.Duration
	execPath string
	args     []string
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	// Check arguments
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	image := req.GetVolumeContext()["image"]

	err := ns.setupVolume(req.GetVolumeId(), image)
	if err != nil {
		glog.V(4).Infof("error: %v", err)
		return nil, err
	}

	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	fsType := req.GetVolumeCapability().GetMount().GetFsType()

	deviceId := ""
	if req.GetPublishContext() != nil {
		deviceId = req.GetPublishContext()[deviceID]
	}

	readOnly := req.GetReadonly()
	volumeId := req.GetVolumeId()
	attrib := req.GetVolumeContext()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	glog.V(4).Infof("target %v\nfstype %v\ndevice %v\nreadonly %v\nvolumeId %v\nattributes %v\n mountflags %v\n",
		targetPath, fsType, deviceId, readOnly, volumeId, attrib, mountFlags)

	options := []string{"bind"}
	if readOnly {
		options = append(options, "ro")
	}

	mounter := mount.New("")
	if err := mounter.Mount(contentPath(volumeId), targetPath, "", options); err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	targetPath := req.GetTargetPath()
	volumeId := req.GetVolumeId()

	// Unmounting the image
	err := mount.New("").Unmount(req.GetTargetPath())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	glog.V(4).Infof("image: volume %s/%s has been unmounted.", targetPath, volumeId)

	err = ns.unsetupVolume(volumeId)
	if err != nil {
		return nil, err
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// TODO: option for extracting image labels
// TODO: option for label to indicate a mount path
// TODO: context
func (ns *nodeServer) setupVolume(volumeId string, image string) error {

	glog.V(4).Infof("pulling %s", image)

	// TODO: imagepullpolicy
	imgService := cri.NewImageServiceClient(ns.criConn)
	_, err := imgService.PullImage(context.TODO(), &cri.PullImageRequest{Image: &cri.ImageSpec{Image: image}})
	if err != nil {
		return err
	}

	// TODO: detect when this is already created, will cause conflicts otherwise
	runService := cri.NewRuntimeServiceClient(ns.criConn)
	pod, err := runService.RunPodSandbox(context.TODO(), &cri.RunPodSandboxRequest{
		Config: &cri.PodSandboxConfig{
			Metadata: &cri.PodSandboxMetadata{
				Name: volumeId,
			},
		},
		RuntimeHandler: "",
	})
	if err != nil {
		glog.V(4).Infof("error creating pod sandbox for %s", image)
		return err
	}

	glog.V(4).Infof("created pod sandbox %s", pod.PodSandboxId)

	cpPath := filepath.Join(utilPath(volumeId), "cp")

	// create a container that mounts in a statically linked `cp` command
	container, err := runService.CreateContainer(context.TODO(), &cri.CreateContainerRequest{
		PodSandboxId: pod.PodSandboxId,
		Config: &cri.ContainerConfig{
			Metadata: &cri.ContainerMetadata{
				Name:    "pull",
				Attempt: 0,
			},
			Image: &cri.ImageSpec{
				Image: image,
			},
			// TODO: allow specifying path within image instead of root
			Command: []string{cpPath, "/", contentPath(volumeId)},
			Mounts: []*cri.Mount{
				{
					ContainerPath: utilPath(volumeId),
					HostPath:      utilPath(volumeId),
					//TODO: is this necessary?
					Propagation: cri.MountPropagation_PROPAGATION_BIDIRECTIONAL,
				},
				{
					ContainerPath: contentPath(volumeId),
					HostPath:      contentPath(volumeId),
				},
			},
		},
		SandboxConfig: &cri.PodSandboxConfig{
			Metadata: &cri.PodSandboxMetadata{
				Name: volumeId + "pull",
			},
		},
	})
	if err != nil {
		return err
	}

	// load in a statically built `cp` that can copy content from within the container
	// this is necessary because there is no way to get a hostPath for a container's fs via CRI
	if err := Copy(cpStaticLocation, cpPath); err != nil {
		return err
	}

	_, err = runService.StartContainer(context.TODO(), &cri.StartContainerRequest{
		ContainerId: container.ContainerId,
	})
	if err != nil {
		return err
	}

	// TODO: wait for container to finish copying, remove container, remove sandbox

	return err
}

func (ns *nodeServer) unsetupVolume(volumeId string) error {
	return os.RemoveAll("/var/csi-image-driver/content/" + volumeId)
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func volumePath(volumeId string) string {
	return filepath.Join("/var/csi-image-driver/volumes/", volumeId)
}

func utilPath(volumeId string) string {
	return filepath.Join(volumePath(volumeId), "util")
}

func contentPath(volumeId string) string {
	return filepath.Join(volumePath(volumeId), "content")
}

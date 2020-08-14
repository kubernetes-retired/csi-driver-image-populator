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
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"k8s.io/kubernetes/pkg/kubelet/util"
)

type driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ids *csicommon.DefaultIdentityServer
	ns  *nodeServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability

	criConn *grpc.ClientConn
}

var (
	version = "0.0.1"
)

func NewDriver(driverName, nodeID, endpoint string) (*driver, error) {
	glog.Infof("Driver: %v version: %v", driverName, version)

	d := &driver{}

	d.endpoint = endpoint

	csiDriver := csicommon.NewCSIDriver(driverName, version, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})
	// image plugin does not support ControllerServiceCapability now.
	// If support is added, it should set to appropriate
	// ControllerServiceCapability RPC types.
	csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_UNKNOWN})

	d.csiDriver = csiDriver

	glog.V(4).Info("establishing CRI connection")

	// TODO: cri endpoint should at least be configurable, or ideally discover the CRI endpoint
	addr, dialer, err := util.GetAddressAndDialer("unix:///var/run/dockershim.sock")
	if err != nil {
		glog.V(4).Info("failed to create CRI dialer")
		return nil, err
	}
	glog.V(4).Info("dialer created")

	// TODO: DialWithContext
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDialer(dialer))
	if err != nil {
		glog.V(4).Info("failed to establish CRI connection")
		return nil, err
	}

	// TODO: context w/ timout
	for {
		state := conn.GetState()
		glog.V(4).Info(state.String())
		if state == connectivity.Ready {
			break
		}
		time.Sleep(1 * time.Second)
	}
	d.criConn = conn

	return d, nil
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
		criConn:           d.criConn,
	}
}

func NewControllerServer(d *csicommon.CSIDriver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
}

func (d *driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(d.endpoint,
		csicommon.NewDefaultIdentityServer(d.csiDriver),
		NewControllerServer(d.csiDriver),
		NewNodeServer(d))
	s.Wait()
}

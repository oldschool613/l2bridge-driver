package l2bridge

import (
	"reflect"

	"github.com/docker/go-plugins-helpers/network"
	"github.com/docker/libnetwork/types"
	"github.com/sirupsen/logrus"
)

type Driver struct {
	bridge *bridgeDriver
}

func NewDriver() *Driver {
	return &Driver{
		bridge: NewBridgeDriver(nil),
	}
}

var capabilities = &network.CapabilitiesResponse{
	Scope:             network.LocalScope,
	ConnectivityScope: network.LocalScope,
}

// unwrap gives the pointed to value if the i is an non-nil pointer.
func unwrap(i interface{}) interface{} {
	if v := reflect.ValueOf(i); v.Kind() == reflect.Ptr && !v.IsNil() {
		return v.Elem()
	}
	return i
}

// logRequest logs request inputs and results.
func logRequest(fname string, req interface{}, res interface{}, err error) {
	req, res = unwrap(req), unwrap(res)
	if err == nil {
		if res == nil {
			logrus.Infof("%s(%v)", fname, req)
		} else {
			logrus.Infof("%s(%v): %v", fname, req, res)
		}
		return
	}
	switch err.(type) {
	case types.MaskableError:
		logrus.WithError(err).Infof("[MaskableError] %s(%v): %v", fname, req, err)
	case types.RetryError:
		logrus.WithError(err).Infof("[RetryError] %s(%v): %v", fname, req, err)
	case types.BadRequestError:
		logrus.WithError(err).Warnf("[BadRequestError] %s(%v): %v", fname, req, err)
	case types.NotFoundError:
		logrus.WithError(err).Warnf("[NotFoundError] %s(%v): %v", fname, req, err)
	case types.ForbiddenError:
		logrus.WithError(err).Warnf("[ForbiddenError] %s(%v): %v", fname, req, err)
	case types.NoServiceError:
		logrus.WithError(err).Warnf("[NoServiceError] %s(%v): %v", fname, req, err)
	case types.NotImplementedError:
		logrus.WithError(err).Warnf("[NotImplementedError] %s(%v): %v", fname, req, err)
	case types.TimeoutError:
		logrus.WithError(err).Errorf("[TimeoutError] %s(%v): %v", fname, req, err)
	case types.InternalError:
		logrus.WithError(err).Errorf("[InternalError] %s(%v): %v", fname, req, err)
	default:
		// Unclassified errors should be treated as bad.
		logrus.WithError(err).Errorf("[UNKNOWN] %s(%v): %v", fname, req, err)
	}
}

func (d *Driver) GetCapabilities() (res *network.CapabilitiesResponse, err error) {
	defer func() { logRequest("GetCapabilities", nil, res, err) }()
	return capabilities, nil
}

func (d *Driver) CreateNetwork(req *network.CreateNetworkRequest) (err error) {
	defer func() { logRequest("CreateNetwork", req, nil, err) }()

	// Convert string IP addresses in the request to net.IPNet.
	ipv4, err := ParseIPAMDataSlice(req.IPv4Data)
	if err != nil {
		return types.BadRequestErrorf("invalid IPv4 information: %v", err)
	}
	ipv6, err := ParseIPAMDataSlice(req.IPv6Data)
	if err != nil {
		return types.BadRequestErrorf("invalid IPv6 information: %v", err)
	}

	// Call into the real bridge driver.
	return d.bridge.CreateNetwork(req.NetworkID, req.Options, ipv4, ipv6)
}

func (d *Driver) AllocateNetwork(req *network.AllocateNetworkRequest) (res *network.AllocateNetworkResponse, err error) {
	defer func() { logRequest("AllocateNetwork", req, res, err) }()
	return nil, types.NotImplementedErrorf("not implemented")
}

func (d *Driver) DeleteNetwork(req *network.DeleteNetworkRequest) (err error) {
	defer func() { logRequest("DeleteNetwork", req, nil, err) }()
	return d.bridge.DeleteNetwork(req.NetworkID)
}

func (d *Driver) FreeNetwork(req *network.FreeNetworkRequest) (err error) {
	defer func() { logRequest("FreeNetwork", req, nil, err) }()
	return types.NotImplementedErrorf("not implemented")
}

func (d *Driver) CreateEndpoint(req *network.CreateEndpointRequest) (res *network.CreateEndpointResponse, err error) {
	defer func() { logRequest("CreateEndpoint", req, res, err) }()

	ei, err := ParseEndpointInterface(req.Interface)
	if err != nil {
		return nil, types.BadRequestErrorf("invalid endpoint info: %v", err)
	}
	ei, err = d.bridge.CreateEndpoint(req.NetworkID, req.EndpointID, ei, req.Options)
	if err != nil {
		return nil, err
	}
	return &network.CreateEndpointResponse{Interface: ei.Marshal()}, nil
}

func (d *Driver) DeleteEndpoint(req *network.DeleteEndpointRequest) (err error) {
	defer func() { logRequest("DeleteEndpoint", req, nil, err) }()
	return d.bridge.DeleteEndpoint(req.NetworkID, req.EndpointID)
}

func (d *Driver) EndpointInfo(req *network.InfoRequest) (res *network.InfoResponse, err error) {
	defer func() { logRequest("EndpointInfo", req, res, err) }()
	info, err := d.bridge.EndpointInfo(req.NetworkID, req.EndpointID)
	if err != nil {
		return nil, err
	}
	return &network.InfoResponse{Value: info}, nil
}

func (d *Driver) Join(req *network.JoinRequest) (res *network.JoinResponse, err error) {
	defer func() { logRequest("Join", req, res, err) }()
	info, err := d.bridge.Join(req.NetworkID, req.EndpointID, req.SandboxKey, req.Options)
	if err != nil {
		return nil, err
	}
	return info.Marshal(), nil
}

func (d *Driver) Leave(req *network.LeaveRequest) (err error) {
	defer func() { logRequest("Leave", req, nil, err) }()
	return d.bridge.Leave(req.NetworkID, req.EndpointID)
}

func (d *Driver) DiscoverNew(notif *network.DiscoveryNotification) (err error) {
	defer func() { logRequest("DiscoverNew", notif, nil, err) }()
	return nil
}

func (d *Driver) DiscoverDelete(notif *network.DiscoveryNotification) (err error) {
	defer func() { logRequest("DiscoverDelete", notif, nil, err) }()
	return nil
}

// ProgramExternalConnectivity is called after Join for non-internal networks to give external network access.
// Although this driver does not support external connectivity, it does not return an error because libnetwork
// will fail the endpoint initialization if any error is returned.
func (d *Driver) ProgramExternalConnectivity(req *network.ProgramExternalConnectivityRequest) (err error) {
	defer func() { logRequest("ProgramExternalConnectivity", req, nil, err) }()
	return nil
}

// RevokeExternalConnectivity is called bedore Leave when tearing down an endpoint to remove up external network access.
// As for ProgramExternalConnectivity, we return no error here, bt take no action.
func (d *Driver) RevokeExternalConnectivity(req *network.RevokeExternalConnectivityRequest) (err error) {
	defer func() { logRequest("RevokeExternalConnectivity", req, nil, err) }()
	return nil
}

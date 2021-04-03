package internal

import (
	"context"
	"net"
	"time"

	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
)

// getHardwareID returns the hardware ID from tink server.
// it will identify if the identifier is an IP or a MAC address
// and make the correct call to tink.
// this hardware ID is what tink uses for the "worker_id". the
// worker_id is used to identify a specific worker and to be able to query
// tink forr things like assigned workflows and actions.
func getHardwareID(ctx context.Context, client hardware.HardwareServiceClient, identifier string) (string, error) {
	var err error
	var hw *hardware.Hardware
	var request hardware.GetRequest
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	switch {
	case isIP(identifier):
		request.Ip = identifier
		hw, err = client.ByIP(ctx, &request)
	case isMac(identifier):
		request.Mac = identifier
		hw, err = client.ByMAC(ctx, &request)
	default:
		err = errors.Errorf("identifier not an IP or MAC address: %v", identifier)
	}

	return hw.GetId(), err
}

func isIP(val string) bool {
	return net.ParseIP(val) != nil
}

func isMac(val string) bool {
	_, err := net.ParseMAC(val)
	return err == nil
}

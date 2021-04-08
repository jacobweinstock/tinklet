package internal

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"google.golang.org/grpc"
)

type hardwareServerMock struct{}

func (h *hardwareServerMock) getMockedHardwareServiceClient() *hardware.HardwareServiceClientMock {
	var hardwareSvcClient hardware.HardwareServiceClientMock

	hardwareSvcClient.ByMACFunc = func(ctx context.Context, in *hardware.GetRequest, opts ...grpc.CallOption) (*hardware.Hardware, error) {
		return &hardware.Hardware{Id: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94"}, nil
	}

	hardwareSvcClient.ByIPFunc = func(ctx context.Context, in *hardware.GetRequest, opts ...grpc.CallOption) (*hardware.Hardware, error) {
		return &hardware.Hardware{Id: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94"}, nil
	}

	return &hardwareSvcClient
}

func TestGetHardwareID(t *testing.T) {
	tests := map[string]struct {
		identifier string
		want       string
		err        error
	}{
		"is ip":              {identifier: "192.168.1.5", want: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94"},
		"is mac":             {identifier: "00:00:00:00:00:00", want: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94"},
		"is not a mac or ip": {identifier: "not good", err: errors.New("identifier not an IP or MAC address: not good")},
	}

	mock := &hardwareServerMock{}
	client := mock.getMockedHardwareServiceClient()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := getHardwareID(context.Background(), client, tc.identifier)
			if err != nil {
				if tc.err != nil {
					if diff := cmp.Diff(err.Error(), tc.err.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
			}
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

package grpcopts

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// toCreds takes a byte string, assumed to be a tls cert, and creates a transport credential
func toCreds(pemCerts []byte) (credentials.TransportCredentials, error) {
	cp := x509.NewCertPool()
	ok := cp.AppendCertsFromPEM(pemCerts)
	if !ok {
		return nil, errors.New("failed to AppendCertsFromPEM")
	}
	return credentials.NewClientTLSFromCert(cp, ""), nil
}

// loadTLSSecureOpts handles taking a string that is assumed to be a boolean
// and creating a grpc.DialOption for TLS.
// If the value is true, the server has a cert from a well known CA.
// If the value is false, the server is not using TLS
func loadTLSSecureOpts(val string) (grpc.DialOption, error) {
	var dialOpt grpc.DialOption
	secure, err := strconv.ParseBool(val)
	if err != nil {
		return nil, errors.WithMessagef(err, "expected boolean, got: %v", val)
	}
	if secure {
		// 1. the server has a cert from a well known CA - grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{}))
	} else {
		// 2. the server is not using TLS - grpc.WithInsecure()
		dialOpt = grpc.WithInsecure()
	}
	return dialOpt, nil
}

// loadTLSFromFile handles reading in a cert file and forming a TLS grpc.DialOption
func loadTLSFromFile(val string) (grpc.DialOption, error) {
	// 3. the server has a self-signed cert and the cert have be provided via file/env/flag -
	u, err := url.Parse(val)
	if err != nil {
		return nil, errors.Wrap(err, "must be file:// schema")
	}
	data, err := os.ReadFile(filepath.Join(u.Host, u.Path))
	if err != nil {
		return nil, err
	}
	creds, err := toCreds(data)
	if err != nil {
		return nil, err
	}
	return grpc.WithTransportCredentials(creds), nil
}

// loadTLSFromHTTP handles reading a cert from an HTTP endpoint and forming a TLS grpc.DialOption
func loadTLSFromHTTP(val string) (grpc.DialOption, error) {
	// 4. the server has a self-signed cert and the cert needs to be grabbed from a URL -
	resp, err := http.Get(val)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	cert, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	creds, err := toCreds([]byte(cert))
	if err != nil {
		return nil, err
	}
	return grpc.WithTransportCredentials(creds), nil
}

// LoadTLSFromValue is the logic for how/from where TLS should be loaded
func LoadTLSFromValue(tlsVal string) (grpc.DialOption, error) {
	u, err := url.Parse(tlsVal)
	if err != nil {
		return nil, errors.Wrap(err, "must be file://, http://, or string boolean")
	}
	switch u.Scheme {
	case "":
		return loadTLSSecureOpts(tlsVal)
	case "file":
		return loadTLSFromFile(tlsVal)
	case "http":
		return loadTLSFromHTTP(tlsVal)
	}
	return nil, fmt.Errorf("not an expected value: %v", tlsVal)
}

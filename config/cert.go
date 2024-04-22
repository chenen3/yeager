package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

// TLS certificate and key in PEM format
type cert struct {
	rootCert   []byte
	rootKey    []byte
	serverCert []byte
	serverKey  []byte
	clientCert []byte
	clientKey  []byte
}

// newCert create TLS certificates for mutual authentication
func newCert(host string) (*cert, error) {
	rootCert, rootKey, err := newRootCA()
	if err != nil {
		return nil, err
	}

	serverCert, serverKey, err := signCert(host, rootCert, rootKey)
	if err != nil {
		return nil, err
	}

	clientCert, clientKey, err := signCert(host, rootCert, rootKey)
	if err != nil {
		return nil, err
	}

	return &cert{
		rootCert:   rootCert,
		rootKey:    rootKey,
		serverCert: serverCert,
		serverKey:  serverKey,
		clientCert: clientCert,
		clientKey:  clientKey,
	}, nil
}

func newRootCA() (certPEM, keyPEM []byte, err error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %s", err)
	}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	rootTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
			CommonName:   "Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &rootTemplate, &rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, nil, err
	}

	b, err := x509.MarshalECPrivateKey(rootKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to marshal ECDSA private key: %s", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return certPEM, keyPEM, nil
}

func signCert(host string, rootCertPEM, rootKeyPEM []byte) (certPEM, keyPEM []byte, err error) {
	cb, _ := pem.Decode(rootCertPEM)
	rootCert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	kb, _ := pem.Decode(rootKeyPEM)
	rootKey, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %s", err)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	hosts := strings.Split(host, ",")
	if len(hosts) == 0 {
		return nil, nil, errors.New("invalid host: " + host)
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, rootCert, &key.PublicKey, rootKey)
	if err != nil {
		return nil, nil, err
	}

	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to marshal ECDSA private key: %s", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return certPEM, keyPEM, nil
}

// newServerTLS creates server-side TLS config for mutual authentication
func newServerTLS(caPEM, certPEM, keyPEM []byte) (*tls.Config, error) {
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(caPEM)
	if !ok {
		return nil, errors.New("failed to parse root cert pem")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.New("parse cert pem: " + err.Error())
	}

	conf := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	return conf, nil
}

// newClientTLS creates client-side TLS config for mutual authentication
func newClientTLS(caPEM, certPEM, keyPEM []byte) (*tls.Config, error) {
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caPEM); !ok {
		return nil, errors.New("parse root certificate")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	conf := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
		Certificates:       []tls.Certificate{cert},
		RootCAs:            pool,
	}
	return conf, nil
}

// MutualTLS creates client and server TLS config for mutual authentication
func MutualTLS(host string) (clientTLS, serverTLS *tls.Config, err error) {
	cert, err := newCert(host)
	if err != nil {
		return
	}
	clientTLS, err = newClientTLS(cert.rootCert, cert.clientCert, cert.clientKey)
	if err != nil {
		return
	}
	serverTLS, err = newServerTLS(cert.rootCert, cert.serverCert, cert.serverKey)
	if err != nil {
		return
	}
	return
}

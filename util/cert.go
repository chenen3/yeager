package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
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

// SelfSignedCertificate generate self-signed certificate, test only
func SelfSignedCertificate() (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return certPEM, keyPEM, nil
}

// Cert certificate and key in PEM format
type Cert struct {
	RootCert   []byte
	RootKey    []byte
	ServerCert []byte
	ServerKey  []byte
	ClientCert []byte
	ClientKey  []byte
}

const (
	CACertFile     = "ca-cert.pem"
	CAKeyFile      = "ca-key.pem"
	ServerCertFile = "server-cert.pem"
	ServerKeyFile  = "server-key.pem"
	ClientCertFile = "client-cert.pem"
	ClientKeyFile  = "client-key.pem"
)

// GenerateCertificate generate TLS certificates for mutual authentication,
// the output files are: ca.key, ca.cert, server.key, server.crt, client.key, client.crt
func GenerateCertificate(host string) (*Cert, error) {
	rootCert, rootKey, err := createRootCA()
	if err != nil {
		return nil, err
	}

	serverCert, serverKey, err := createCertificate(host, rootCert, rootKey)
	if err != nil {
		return nil, err
	}

	clientCert, clientKey, err := createCertificate(host, rootCert, rootKey)
	if err != nil {
		return nil, err
	}

	return &Cert{
		RootCert:   rootCert,
		RootKey:    rootKey,
		ServerCert: serverCert,
		ServerKey:  serverKey,
		ClientCert: clientCert,
		ClientKey:  clientKey,
	}, nil
}

func createRootCA() (certPEM, keyPEM []byte, err error) {
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

func createCertificate(host string, rootCertPEM, rootKeyPEM []byte) (certPEM, keyPEM []byte, err error) {
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

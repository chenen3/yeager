package grpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"testing"
)

func TestGRPC(t *testing.T) {
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	tlsConf := &tls.Config{
		Certificates:             []tls.Certificate{cert},
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
	}
	lis, err := Listen("127.0.0.1:0", tlsConf)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	go func() {
		conn, err := lis.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	d := NewDialer(lis.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := d.DialContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := []byte{1}
	_, err = conn.Write(want)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1)
	_, err = conn.Read(got[:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// how to generate TLS certificate and private key:
// go run $GOROOT/src/crypto/tls/generate_cert.go --host localhost
var certPEM = `-----BEGIN CERTIFICATE-----
MIIC+TCCAeGgAwIBAgIQDb5nH79/oPWE8qsGrcedHzANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMB4XDTIxMDMyMTA5MDkyNFoXDTIyMDMyMTA5MDky
NFowEjEQMA4GA1UEChMHQWNtZSBDbzCCASIwDQYJKoZIhvcNAQEBBQADggEPADCC
AQoCggEBAOIaGuvjZv8Y9YXKZdtMY0WGpawFQHnp+jJml29aeQeCNnYWjUUGOFO+
zXPwyKCcPjcpfeJ/FULhbmKyXwRdddYrpIo53TPEHyYvlvGDAVhPX1HIAJDRyGFu
axcZThvaxcTMxFrUylRtuusBy5FwdL1MiyM9UQrgXiY5dKILxpQ34aWSb2bBdMUV
BTnVz3Gx/ewUZwMioGJIlh59SciMrNhNQxjnJ8lHGUU6ue3GBivcCGMa4GXQtMn2
a8YVhwaRDwqx9mRgzK4nf4iJvxXncaJQ3O7qiBlonQcm88Vp7EbnrDEQh7WbRg0h
6/x0mwFTn7BGSwMvmt7+fw4w4xwQ5aMCAwEAAaNLMEkwDgYDVR0PAQH/BAQDAgWg
MBMGA1UdJQQMMAoGCCsGAQUFBwMBMAwGA1UdEwEB/wQCMAAwFAYDVR0RBA0wC4IJ
bG9jYWxob3N0MA0GCSqGSIb3DQEBCwUAA4IBAQAJyz4VvjmwgIUZJtNoBKmEkexe
FPEU8ZzLRUvTaOyJMKHfcOTTaLtqRaFPrbH9UrVhDnyhaycTmloosqG2yqEUJpQc
WnE7XbdshY4TSD0q3+nF7qwCsOlHNNIOPC2FPJAWrBpWtf2o0oDv6BmpOQsJT8Vd
kBYJmMkQWiMD0xEcLoW7h1RCJfY2uGyTc7Vqnhv3zf2eJhJQW9GneTkqZiuIEgmo
JP41TMXN0anvHUyGGtj5EuvYYpl1UdaYw7HfBc9PcjIXa3ZqyP+Rwr+893/wYXEr
GPbNxX5umBN+RL6Xo7gNwhpz/b6hmjeVlo/UPDXr3ifc87/4NPe2tV+z34X8
-----END CERTIFICATE-----
`

var keyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDiGhrr42b/GPWF
ymXbTGNFhqWsBUB56foyZpdvWnkHgjZ2Fo1FBjhTvs1z8MignD43KX3ifxVC4W5i
sl8EXXXWK6SKOd0zxB8mL5bxgwFYT19RyACQ0chhbmsXGU4b2sXEzMRa1MpUbbrr
AcuRcHS9TIsjPVEK4F4mOXSiC8aUN+Glkm9mwXTFFQU51c9xsf3sFGcDIqBiSJYe
fUnIjKzYTUMY5yfJRxlFOrntxgYr3AhjGuBl0LTJ9mvGFYcGkQ8KsfZkYMyuJ3+I
ib8V53GiUNzu6ogZaJ0HJvPFaexG56wxEIe1m0YNIev8dJsBU5+wRksDL5re/n8O
MOMcEOWjAgMBAAECggEBAJB3AZCKwbpopieQBLiz/BHmUGCzEllTKGufpU6ezgoA
JvAYxLa/nKnVlcxooqgMbKwuNKLNdDZBd5hUQ+W0GaB4Ti+LfUygGQ77BUTW4bsd
K9hDJClkANZQCNi/cGrXw9lUxHkM0SJU4kNZ6EnLQqvZBmXDvc5nQYDG4UDQqVb1
kiM1fG2oO3vkITKmuorRRM0TCQsIhM3dorx7tvuaKfNF0eIlynCx4JCeGKppb6oM
HPvwkcEHZ4Ym7vXxrwQQh+qiF7FPZlCP7lVM/HPXZy9mvt0Dx//KGEiRE0LxI/10
tvkHAbFbtp3W7Dvb0A30ZIyWoKIqMeBr7wzCCFgDi3ECgYEA77Bl5pYbd8jYOSsC
vZH9WImZI2j3bJB9KeLT4EAy9ZeC1gQKTDofrWXBxu9V2HhGDJsg4D0PlZd3cfmY
vxkNEJG+EqcijgHvewWhNbYshGTrgn3J3Ko6Rw3bK6Fg03X1lSrO/cybXvC69BNT
QTW1g3IkSWTluFtjrViadOuJlDUCgYEA8X0B/HPXGQV7zAdXiBOZTvYHEZSbqzch
b+mmxkFdYv/K5VkXnibE0FpKS3RLjsasf+hR89MPM3wc6Dw3GtE2Zo4eK0vUX0VD
wA5jP/ghXy4G3DXPSq71EDnfHr7gt5grsIBVAx2hAgmyjmXSklwRB8GXkB75cS1B
+tl3u8hnHXcCgYA1PjwEksebPjQ5zsIXFjzu0/H+mayMozQKf+aM4/Xt9DAOFmur
LyYmQHphFH0/TshQuIz/AtFZa4IPAWDa4leynI1aW2IjpW5rJ37+DW+qITjnjcWv
jOjRK9TJxllZ39QjxJSicDb7SgJdgYV28NVXU52X6B/XagWkVhBJdKDlGQKBgFu4
m1SDuyMpzgeEkl6A8y3mjHDE/Qte+ThEiq+qjAnaFfpeiHXtS7vHT4ixNzGXjFVY
rCfr9k4bye77UALDi+IQAK15M8SrzjvYOyJE4IgCN2DUn1NCeJodIP3QihGxnoZ/
d8qjKlBX1pX3Xq9wgJdtlF+NJDk0c2cPykZsq52pAoGADwlyChkVz6yPlgcUGlR4
xBjXUz43N3Qe6pVzi4Iuw82W19KxTMjKJYMslMsEuAFJNlhU0FdIvLgdHmvcdmr5
BEtxO7vnx7y8uqHR8zif54KFVS0C8vAB1bpjdkXSHmppQcDEsZzaVMhBwDH36tEJ
FrU7u8m1bVe7FzwsfDLbsTw=
-----END RSA PRIVATE KEY-----
`

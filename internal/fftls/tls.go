package fftls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/i18n"
)

// InitTLSConfig initializes TLS configuration with secure defaults
func InitTLSConfig(conf config.Section) {
	conf.AddKnownKey(ConfigTLSEnabled, true)
	conf.AddKnownKey(ConfigTLSClientAuth, "require")
	conf.AddKnownKey(ConfigTLSMinVersion, "1.2")
	conf.AddKnownKey(ConfigTLSMaxVersion, "1.3")
	conf.AddKnownKey(ConfigTLSCertFile)
	conf.AddKnownKey(ConfigTLSKeyFile)
	conf.AddKnownKey(ConfigTLSCAFile)
	conf.AddKnownKey(ConfigTLSInsecureSkipVerify, false)
	conf.AddKnownKey(ConfigTLSRenegotiation, false)
	conf.AddKnownKey(ConfigTLSVerifyHostname, true)
	conf.AddKnownKey(ConfigTLSVerifyPeerCertificate, true)
}

// ConfigTLS creates a TLS configuration with secure defaults
func ConfigTLS(conf config.Section) (*tls.Config, error) {
	if !conf.GetBool(ConfigTLSEnabled) {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		PreferServerCipherSuites: true,
		InsecureSkipVerify:       conf.GetBool(ConfigTLSInsecureSkipVerify),
		Renegotiation:            tls.RenegotiateNever,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if conf.GetBool(ConfigTLSVerifyPeerCertificate) {
				opts := x509.VerifyOptions{
					DNSName:       cs.ServerName,
					Intermediates: x509.NewCertPool(),
				}
				for _, cert := range cs.PeerCertificates[1:] {
					opts.Intermediates.AddCert(cert)
				}
				_, err := cs.PeerCertificates[0].Verify(opts)
				return err
			}
			return nil
		},
	}

	// Load certificate and key
	certFile := conf.GetString(ConfigTLSCertFile)
	keyFile := conf.GetString(ConfigTLSKeyFile)
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, i18n.WrapError(nil, err, i18n.MsgFailedToLoadTLSKeyPair)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate
	caFile := conf.GetString(ConfigTLSCAFile)
	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, i18n.WrapError(nil, err, i18n.MsgFailedToLoadTLSCACert)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, i18n.NewError(nil, i18n.MsgFailedToAppendTLSCACert)
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Configure client authentication
	switch conf.GetString(ConfigTLSClientAuth) {
	case "require":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	case "verify-if-given":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case "none":
		tlsConfig.ClientAuth = tls.NoClientCert
	default:
		return nil, i18n.NewError(nil, i18n.MsgInvalidTLSClientAuth)
	}

	// Set minimum TLS version
	switch conf.GetString(ConfigTLSMinVersion) {
	case "1.2":
		tlsConfig.MinVersion = tls.VersionTLS12
	case "1.3":
		tlsConfig.MinVersion = tls.VersionTLS13
	default:
		return nil, i18n.NewError(nil, i18n.MsgInvalidTLSMinVersion)
	}

	// Set maximum TLS version
	switch conf.GetString(ConfigTLSMaxVersion) {
	case "1.2":
		tlsConfig.MaxVersion = tls.VersionTLS12
	case "1.3":
		tlsConfig.MaxVersion = tls.VersionTLS13
	default:
		return nil, i18n.NewError(nil, i18n.MsgInvalidTLSMaxVersion)
	}

	return tlsConfig, nil
}

// ValidateTLSConfig validates TLS configuration
func ValidateTLSConfig(conf config.Section) error {
	if !conf.GetBool(ConfigTLSEnabled) {
		return nil
	}

	// Check certificate files
	certFile := conf.GetString(ConfigTLSCertFile)
	keyFile := conf.GetString(ConfigTLSKeyFile)
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return i18n.NewError(nil, i18n.MsgTLSFilesRequired)
		}
		if _, err := os.Stat(certFile); err != nil {
			return i18n.WrapError(nil, err, i18n.MsgTLSCertFileNotFound)
		}
		if _, err := os.Stat(keyFile); err != nil {
			return i18n.WrapError(nil, err, i18n.MsgTLSKeyFileNotFound)
		}

		// Check certificate permissions
		if info, err := os.Stat(certFile); err == nil {
			if info.Mode().Perm()&0077 != 0 {
				return i18n.NewError(nil, i18n.MsgTLSCertFilePermissions)
			}
		}
		if info, err := os.Stat(keyFile); err == nil {
			if info.Mode().Perm()&0077 != 0 {
				return i18n.NewError(nil, i18n.MsgTLSKeyFilePermissions)
			}
		}
	}

	// Check CA file
	caFile := conf.GetString(ConfigTLSCAFile)
	if caFile != "" {
		if _, err := os.Stat(caFile); err != nil {
			return i18n.WrapError(nil, err, i18n.MsgTLSCAFileNotFound)
		}
	}

	return nil
} 
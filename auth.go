// Copyright (c) 2019 Blacknon. All rights reserved.
// Use of this source code is governed by an MIT license
// that can be found in the LICENSE file.

// TODO(blacknon):
//     ↓等を読み解いて、Publickey offeringやknown_hostsのチェックを実装する。
//     既存のライブラリ等はないので、自前でrequestを書く必要があるかも？
//     かなりの手間がかかりそうなので、対応については相応に時間がかかりそう。
//       - https://go.googlesource.com/crypto/+/master/ssh/client_auth.go
//       - https://go.googlesource.com/crypto/+/master/ssh/tcpip.go

package sshlib

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/miekg/pkcs11/p11"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// CreateAuthMethodPassword returns ssh.AuthMethod generated from password.
func CreateAuthMethodPassword(password string) (auth ssh.AuthMethod) {
	return ssh.Password(password)
}

// CreateAuthMethodPublicKey returns ssh.AuthMethod generated from PublicKey.
// If you have not specified a passphrase, please specify a empty character("").
func CreateAuthMethodPublicKey(key, password string) (auth ssh.AuthMethod, err error) {
	signer, err := CreateSignerPublicKey(key, password)
	if err != nil {
		return
	}

	auth = ssh.PublicKeys(signer)
	return
}

// CreateSignerPublicKey returns []ssh.Signer generated from public key.
// If you have not specified a passphrase, please specify a empty character("").
func CreateSignerPublicKey(key, password string) (signer ssh.Signer, err error) {
	// get absolute path
	key = getAbsPath(key)

	// Read PrivateKey file
	keyData, err := ioutil.ReadFile(key)
	if err != nil {
		return
	}

	signer, err = CreateSignerPublicKeyData(keyData, password)

	return
}

// CreateSignerPublicKeyData return ssh.Signer from private key and password
func CreateSignerPublicKeyData(keyData []byte, password string) (signer ssh.Signer, err error) {
	if password != "" { // password is not empty
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(password))
	} else { // password is empty
		signer, err = ssh.ParsePrivateKey(keyData)
	}

	return
}

// CreateSignerPublicKeyPrompt rapper CreateSignerPKCS11.
// Output a passphrase input prompt if the passphrase is not entered or incorrect.
//
// Only Support UNIX-like OS.
func CreateSignerPublicKeyPrompt(key, password string) (signer ssh.Signer, err error) {
	// repeat count
	rep := 3

	// get absolute path
	key = getAbsPath(key)

	// Read PrivateKey file
	keyData, err := ioutil.ReadFile(key)
	if err != nil {
		return
	}

	if password != "" { // password is not empty
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(password))
	} else { // password is empty
		signer, err = ssh.ParsePrivateKey(keyData)

		rgx := regexp.MustCompile(`cannot decode`)
		if err != nil {
			if rgx.MatchString(err.Error()) {
				msg := key + "'s passphrase:"

				for i := 0; i < rep; i++ {
					password, _ = getPassphrase(msg)
					password = strings.TrimRight(password, "\n")
					signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(password))
					if err == nil {
						return
					}
					fmt.Println("\n" + err.Error())
				}
			}
		}
	}

	return
}

// CreateAuthMethodCertificate returns ssh.AuthMethod generated from Certificate.
// To generate an AuthMethod from a certificate, you will need the certificate's private key Signer.
// Signer should be generated from CreateSignerPublicKey() or CreateSignerPKCS11().
func CreateAuthMethodCertificate(cert string, keySigner ssh.Signer) (auth ssh.AuthMethod, err error) {
	signer, err := CreateSignerCertificate(cert, keySigner)
	if err != nil {
		return
	}

	auth = ssh.PublicKeys(signer)
	return
}

// CreateSignerCertificate returns ssh.Signer generated from Certificate.
// To generate an AuthMethod from a certificate, you will need the certificate's private key Signer.
// Signer should be generated from CreateSignerPublicKey() or CreateSignerPKCS11().
func CreateSignerCertificate(cert string, keySigner ssh.Signer) (certSigner ssh.Signer, err error) {
	// get absolute path
	cert = getAbsPath(cert)

	// Read Cert file
	certData, err := ioutil.ReadFile(cert)
	if err != nil {
		return
	}

	// Create PublicKey from Cert
	pubkey, _, _, _, err := ssh.ParseAuthorizedKey(certData)
	if err != nil {
		return
	}

	// Create Certificate Struct
	certificate, ok := pubkey.(*ssh.Certificate)
	if !ok {
		err = fmt.Errorf("%s\n", "Error: Not create certificate struct data")
		return
	}

	// Create Certificate Signer
	certSigner, err = ssh.NewCertSigner(certificate, keySigner)
	if err != nil {
		return
	}

	return
}

// CreateAuthMethodPKCS11 return []ssh.AuthMethod generated from pkcs11 token.
// PIN is required to generate a AuthMethod from a PKCS 11 token.
//
// WORNING: Does not work if multiple tokens are stuck at the same time.
func CreateAuthMethodPKCS11(provider, pin string) (auth []ssh.AuthMethod, err error) {
	signers, err := CreateSignerPKCS11(provider, pin)
	if err != nil {
		return
	}

	for _, signer := range signers {
		auth = append(auth, ssh.PublicKeys(signer))
	}
	return
}

// CreateSignerPKCS11 returns []ssh.Signer generated from PKCS11 token.
// PIN is required to generate a Signer from a PKCS 11 token.
//
// WORNING: Does not work if multiple tokens are stuck at the same time.
func CreateSignerPKCS11(provider, pin string) (signers []ssh.Signer, err error) {
	// get absolute path
	provider = getAbsPath(provider)

	// Create p11.module
	module, err := p11.OpenModule(provider)
	if err != nil {
		return
	}

	// Get p11 Module's Slot
	slots, err := module.Slots()
	if err != nil {
		return
	}
	c11array := []*C11{}

	for _, slot := range slots {
		tokenInfo, err := slot.TokenInfo()
		if err != nil {
			continue
		}

		c := &C11{
			Label: tokenInfo.Label,
			PIN:   pin,
		}
		c11array = append(c11array, c)
	}

	// Destroy Module
	module.Destroy()

	// for loop
	for _, c11 := range c11array {
		err := c11.CreateCtx(provider)
		if err != nil {
			continue
		}

		sigs, err := c11.GetSigner()
		if err != nil {
			continue
		}

		for _, sig := range sigs {
			signer, _ := ssh.NewSignerFromSigner(sig)
			signers = append(signers, signer)
		}
	}

	return
}

// CreateSignerAgent return []ssh.Signer from ssh-agent.
// In sshAgent, put agent.Agent or agent.ExtendedAgent.
func CreateSignerAgent(sshAgent interface{}) (signers []ssh.Signer, err error) {
	switch ag := sshAgent.(type) {
	case agent.Agent:
		signers, err = ag.Signers()
	case agent.ExtendedAgent:
		signers, err = ag.Signers()
	}

	return
}

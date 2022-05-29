// Copyright (c) 2021 Blacknon. All rights reserved.
// Use of this source code is governed by an MIT license
// that can be found in the LICENSE file.
//go:build cgo
// +build cgo

package sshlib

import (
	"github.com/miekg/pkcs11/p11"
	"golang.org/x/crypto/ssh"
)

// CreateAuthMethodPKCS11 return []ssh.AuthMethod generated from pkcs11 token.
// PIN is required to generate a AuthMethod from a PKCS 11 token.
// Not available if cgo is disabled.
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
// Not available if cgo is disabled.
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
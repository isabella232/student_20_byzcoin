package crypto

import (
	"errors"

	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/random"
)

// SchnorrSig is a signature created using the Schnorr Signature scheme.
type SchnorrSig struct {
	Challenge abstract.Secret
	Response  abstract.Secret
}

// MarshalBinary is used for example in hashing
func (ss *SchnorrSig) MarshalBinary() ([]byte, error) {
	cbuf, err := ss.Challenge.MarshalBinary()
	if err != nil {
		return nil, err
	}
	rbuf, err := ss.Response.MarshalBinary()
	return append(cbuf, rbuf...), err
}

// SignSchnorr creates a Schnorr signature from a msg and a private key
func SignSchnorr(suite abstract.Suite, private abstract.Secret, msg []byte) (SchnorrSig, error) {
	// using notation from https://en.wikipedia.org/wiki/Schnorr_signature
	// create random secret k and public point commitment r
	k := suite.Secret().Pick(random.Stream)
	r := suite.Point().Mul(nil, k)

	// create challenge e based on message and r
	e, err := hash(suite, r, msg)
	if err != nil {
		return SchnorrSig{}, err
	}

	// compute response s = k - x*e
	xe := suite.Secret().Mul(private, e)
	s := suite.Secret().Sub(k, xe)

	return SchnorrSig{Challenge: e, Response: s}, nil
}

// VerifySchnorr verifies a given Schnorr signature. It returns nil iff the given signature is valid.
func VerifySchnorr(suite abstract.Suite, public abstract.Point, msg []byte, sig SchnorrSig) error {
	// compute rv = g^s * y^e (where y = g^x)
	gs := suite.Point().Mul(nil, sig.Response)
	ye := suite.Point().Mul(public, sig.Challenge)
	rv := suite.Point().Add(gs, ye)

	// recompute challenge (e) from rv
	e, err := hash(suite, rv, msg)
	if err != nil {
		return err
	}

	if !e.Equal(sig.Challenge) {
		return errors.New("Signature not valid: Reconstructed challenge isn't equal to challenge in signature")
	}

	return nil
}

func hash(suite abstract.Suite, r abstract.Point, msg []byte) (abstract.Secret, error) {
	rBuf, err := r.MarshalBinary()
	if err != nil {
		return nil, err
	}
	cipher := suite.Cipher(rBuf)
	cipher.Message(nil, nil, msg)
	// (re)compute challenge (e)
	e := suite.Secret().Pick(cipher)

	return e, nil
}
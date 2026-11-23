package registrar

import (
	"errors"
	"testing"
	"time"

	"github.com/fil-forge/libforge/identity"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/did/key"
	"github.com/fil-forge/ucantone/did/resolver"
	"github.com/fil-forge/ucantone/multikey"
	"github.com/fil-forge/ucantone/multikey/ed25519"
)

func TestSignedByDID(t *testing.T) {
	newKey := func(t *testing.T) multikey.Signer {
		t.Helper()
		s, err := ed25519.Generate()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	webDID, err := did.Parse("did:web:node.example")
	if err != nil {
		t.Fatal(err)
	}
	// documentOf returns the DID document a did:web node with this key serves.
	documentOf := func(t *testing.T, k multikey.Signer) did.Document {
		t.Helper()
		doc, err := identity.Identity{Issuer: multikey.NewIssuer(webDID, k)}.DIDDocument()
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	msg := []byte(webDID.String())

	t.Run("a did:key signs for itself", func(t *testing.T) {
		issuer := multikey.KeyIssuer(newKey(t))
		svc := &Service{resolver: key.Resolver}
		m := []byte(issuer.DID().String())

		ok, err := svc.signedByDID(t.Context(), issuer.DID(), m, issuer.Sign(m))
		if err != nil || !ok {
			t.Fatalf("want signed, got %v, %v", ok, err)
		}
	})

	t.Run("the key in a did:web document signs for it", func(t *testing.T) {
		k := newKey(t)
		svc := &Service{resolver: resolver.WellKnown{webDID: documentOf(t, k)}}

		ok, err := svc.signedByDID(t.Context(), webDID, msg, k.Sign(msg))
		if err != nil || !ok {
			t.Fatalf("want signed, got %v, %v", ok, err)
		}
	})

	t.Run("a key the document does not list does not", func(t *testing.T) {
		svc := &Service{resolver: resolver.WellKnown{webDID: documentOf(t, newKey(t))}}

		ok, err := svc.signedByDID(t.Context(), webDID, msg, newKey(t).Sign(msg))
		if err != nil || ok {
			t.Fatalf("want not signed, got %v, %v", ok, err)
		}
	})

	t.Run("an expired key does not", func(t *testing.T) {
		oldKey, newKey := newKey(t), newKey(t)
		doc := documentOf(t, newKey)
		expired := did.DateTimeStamp(time.Now().Add(-time.Hour))
		vm := multikey.DeriveVerificationMethod(doc.Fragment("key-1"), oldKey.Verifier().(multikey.Verifier))
		vm.Controller = doc.ID
		vm.Expires = &expired
		if err := doc.VerificationMethods.Add(vm); err != nil {
			t.Fatal(err)
		}
		if err := doc.Authentication.Add(vm); err != nil {
			t.Fatal(err)
		}
		svc := &Service{resolver: resolver.WellKnown{webDID: doc}}

		ok, err := svc.signedByDID(t.Context(), webDID, msg, oldKey.Sign(msg))
		if err != nil || ok {
			t.Fatalf("want not signed by the expired key, got %v, %v", ok, err)
		}
		ok, err = svc.signedByDID(t.Context(), webDID, msg, newKey.Sign(msg))
		if err != nil || !ok {
			t.Fatalf("want signed by the current key, got %v, %v", ok, err)
		}
	})

	t.Run("a key the document lists for another purpose does not", func(t *testing.T) {
		current, other := newKey(t), newKey(t)
		doc := documentOf(t, current)
		vm := multikey.DeriveVerificationMethod(doc.Fragment("key-1"), other.Verifier().(multikey.Verifier))
		vm.Controller = doc.ID
		if err := doc.VerificationMethods.Add(vm); err != nil {
			t.Fatal(err)
		}
		if err := doc.AssertionMethod.Add(vm); err != nil {
			t.Fatal(err)
		}
		svc := &Service{resolver: resolver.WellKnown{webDID: doc}}

		ok, err := svc.signedByDID(t.Context(), webDID, msg, other.Sign(msg))
		if err != nil || ok {
			t.Fatalf("want not signed, got %v, %v", ok, err)
		}
	})

	t.Run("a document with no authentication keys authenticates nothing", func(t *testing.T) {
		k := newKey(t)
		doc := documentOf(t, k)
		doc.Authentication = nil
		svc := &Service{resolver: resolver.WellKnown{webDID: doc}}

		ok, err := svc.signedByDID(t.Context(), webDID, msg, k.Sign(msg))
		if err != nil || ok {
			t.Fatalf("want not signed, got %v, %v", ok, err)
		}
	})

	t.Run("a document for another DID is refused", func(t *testing.T) {
		k := newKey(t)
		otherDID, err := did.Parse("did:web:other.example")
		if err != nil {
			t.Fatal(err)
		}
		doc, err := identity.Identity{Issuer: multikey.NewIssuer(otherDID, k)}.DIDDocument()
		if err != nil {
			t.Fatal(err)
		}
		svc := &Service{resolver: resolver.WellKnown{webDID: doc}}

		_, err = svc.signedByDID(t.Context(), webDID, msg, k.Sign(msg))
		if !errors.Is(err, ErrInvalidDID) {
			t.Fatalf("want ErrInvalidDID, got %v", err)
		}
	})

	t.Run("a DID that does not resolve is an error", func(t *testing.T) {
		svc := &Service{resolver: resolver.WellKnown{}}

		if _, err := svc.signedByDID(t.Context(), webDID, msg, newKey(t).Sign(msg)); err == nil {
			t.Fatal("want an error for an unresolvable DID")
		} else if errors.Is(err, ErrInvalidDID) {
			t.Fatalf("a failed resolution does not make the DID invalid, got %v", err)
		}
	})
}

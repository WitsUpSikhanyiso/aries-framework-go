/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aries

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/aries-framework-go/pkg/didmethod/peer"
	"github.com/hyperledger/aries-framework-go/pkg/storage/leveldb"

	"github.com/hyperledger/aries-framework-go/pkg/didcomm/protocol/exchange"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/transport"
	"github.com/hyperledger/aries-framework-go/pkg/doc/did"
	"github.com/hyperledger/aries-framework-go/pkg/framework/didresolver"
	mocktransport "github.com/hyperledger/aries-framework-go/pkg/internal/didcomm/transport/mock"
	"github.com/stretchr/testify/require"
	errors "golang.org/x/xerrors"
)

var doc = `{
  "@context": ["https://w3id.org/did/v1","https://w3id.org/did/v2"],
  "id": "did:peer:21tDAKCERh95uGgKbJNHYp",
  "publicKey": [
    {
      "id": "did:peer:123456789abcdefghi#keys-1",
      "type": "Secp256k1VerificationKey2018",
      "controller": "did:peer:123456789abcdefghi",
      "publicKeyBase58": "H3C2AVvLMv6gmMNam3uVAjZpfkcJCwDwnZn6z3wXmqPV"
    },
    {
      "id": "did:peer:123456789abcdefghw#key2",
      "type": "RsaVerificationKey2018",
      "controller": "did:peer:123456789abcdefghw",
      "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAryQICCl6NZ5gDKrnSztO\n3Hy8PEUcuyvg/ikC+VcIo2SFFSf18a3IMYldIugqqqZCs4/4uVW3sbdLs/6PfgdX\n7O9D22ZiFWHPYA2k2N744MNiCD1UE+tJyllUhSblK48bn+v1oZHCM0nYQ2NqUkvS\nj+hwUU3RiWl7x3D2s9wSdNt7XUtW05a/FXehsPSiJfKvHJJnGOX0BgTvkLnkAOTd\nOrUZ/wK69Dzu4IvrN4vs9Nes8vbwPa/ddZEzGR0cQMt0JBkhk9kU/qwqUseP1QRJ\n5I1jR4g8aYPL/ke9K35PxZWuDp3U0UPAZ3PjFAh+5T+fc7gzCs9dPzSHloruU+gl\nFQIDAQAB\n-----END PUBLIC KEY-----"
    }
  ]
}`

func TestFramework(t *testing.T) {
	t.Run("test framework new - returns error", func(t *testing.T) {
		path, cleanup := setupLevelDB(t)
		defer cleanup()
		dbPath = path

		// framework new - error
		_, err := New(func(opts *Aries) error {
			return errors.New("error creating the framework option")
		})
		require.Error(t, err)
	})

	// framework new - success
	t.Run("test framework new - returns framework", func(t *testing.T) {
		path, cleanup := setupLevelDB(t)
		defer cleanup()
		dbPath = path
		aries, err := New(WithTransportProviderFactory(&mockTransportProviderFactory{}))
		require.NoError(t, err)

		// context
		ctx, err := aries.Context()
		require.NoError(t, err)

		// exchange client
		exClient := exchange.New(ctx)
		require.NoError(t, err)

		req := &exchange.Request{
			ID:    "5678876542345",
			Label: "Bob",
		}
		require.NoError(t, exClient.SendExchangeRequest(req, "http://example/didexchange"))
		require.Error(t, exClient.SendExchangeRequest(req, ""))
	})

	// framework new - success
	t.Run("test DID resolver - with user provided resolver", func(t *testing.T) {
		peerDID := "did:peer:123"
		// with consumer provider DID resolver
		aries, err := New(WithDIDResolver(didresolver.New(didresolver.WithDidMethod(mockDidMethod{readValue: []byte(doc), acceptFunc: func(method string) bool {
			return method == "peer"
		}}))))
		require.NoError(t, err)
		require.NotEmpty(t, aries)

		resolvedDoc, err := aries.DIDResolver().Resolve(peerDID)
		require.NoError(t, err)
		originalDoc, err := did.FromBytes([]byte(doc))
		require.NoError(t, err)

		require.Equal(t, originalDoc, resolvedDoc)
		err = aries.Close()
		require.NoError(t, err)
	})

	// framework new - success
	t.Run("test DID resolver - with default resolver", func(t *testing.T) {
		// store peer DID in the store
		dbprov, err := leveldb.NewProvider(dbPath)
		require.NoError(t, err)

		dbstore, err := dbprov.GetStoreHandle()
		require.NoError(t, err)

		peerDID := "did:peer:21tDAKCERh95uGgKbJNHYp"
		store := peer.NewDIDStore(dbstore)
		originalDoc, err := did.FromBytes([]byte(doc))
		require.NoError(t, err)
		err = store.Put(peerDID, originalDoc, nil)
		require.NoError(t, err)

		err = dbprov.Close()
		require.NoError(t, err)

		// with default DID resolver
		aries, err := New()
		require.NoError(t, err)
		require.NotEmpty(t, aries)

		resolvedDoc, err := aries.DIDResolver().Resolve(peerDID)
		require.NoError(t, err)
		require.Equal(t, originalDoc, resolvedDoc)
		err = aries.Close()
		require.NoError(t, err)
	})

}

type mockTransportProviderFactory struct {
}

func (f *mockTransportProviderFactory) CreateOutboundTransport() transport.OutboundTransport {
	return mocktransport.NewOutboundTransport("success")
}

type mockDidMethod struct {
	readValue  []byte
	readErr    error
	acceptFunc func(method string) bool
}

func (m mockDidMethod) Read(did string, opts ...didresolver.ResolveOpt) ([]byte, error) {
	return m.readValue, m.readErr
}

func (m mockDidMethod) Accept(method string) bool {
	return m.acceptFunc(method)
}

func setupLevelDB(t testing.TB) (string, func()) {
	path, err := ioutil.TempDir("", "db")
	if err != nil {
		t.Fatalf("Failed to create leveldb directory: %s", err)
	}
	return path, func() {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatalf("Failed to clear leveldb directory: %s", err)
		}
	}
}

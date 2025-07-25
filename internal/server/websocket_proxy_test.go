package server

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1"
	mocks "git.netflux.io/rob/octoplex/internal/generated/mocks/server"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"git.netflux.io/rob/octoplex/internal/token"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestWebSocketProxyCredentialsDisabled(t *testing.T) {
	const rawToken = "s3cr3t"

	sessionToken, err := token.New(token.RawToken(rawToken), time.Time{})
	require.NoError(t, err)

	testCases := []struct {
		name            string
		tokenStoreFunc  func(t *testing.T, tokenStore *mocks.TokenStore)
		credentialsMode CredentialsMode
		cookieHeader    string
		wantRecvErr     string
	}{
		{
			name:            "credentials disabled",
			credentialsMode: CredentialsModeDisabled,
		},
		{
			name:            "credentials enabled, no credentials provided",
			credentialsMode: CredentialsModeEnabled,
			wantRecvErr:     "websocket: close 4003: unauthorized",
		},
		{
			name: "credentials enabled, credentials provided",
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get("session-token").Return(sessionToken, nil)
				tokenStore.EXPECT().Put("session-token", mock.MatchedBy(func(t domain.Token) bool {
					return t.ExpiresAt.After(time.Now().Add(cookieValidFor - (5 * time.Second)))
				})).Return(nil)
			},
			credentialsMode: CredentialsModeEnabled,
			cookieHeader:    "octoplex-session=" + hex.EncodeToString([]byte(rawToken)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := testhelpers.NewTestLogger(t)

			var tokenStore mocks.TokenStore
			defer tokenStore.AssertExpectations(t)

			if tc.tokenStoreFunc != nil {
				tc.tokenStoreFunc(t, &tokenStore)
			}

			proxy := newWebSocketProxy(
				config.Config{},
				newServer(
					func(event.Command) (event.Event, error) { panic("not implemented") },
					func(event.ClientID, event.Command) { panic("not implemented") },
					event.NewBus(logger),
					logger,
				),
				tc.credentialsMode,
				&tokenStore,
				logger,
			)

			srv := httptest.NewServer(proxy.Handler())
			t.Cleanup(srv.Close)

			hdr := http.Header{}
			if tc.cookieHeader != "" {
				hdr.Add("cookie", tc.cookieHeader)
			}
			conn, resp, err := websocket.DefaultDialer.Dial(strings.Replace(srv.URL, "http://", "ws://", 1)+"/ws", hdr)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			t.Cleanup(func() { _ = conn.Close() })

			require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
			require.NoError(t, conn.SetWriteDeadline(time.Now().Add(5*time.Second)))

			startHandshakeCmd, err := proto.Marshal(&pb.Envelope{Payload: &pb.Envelope_Command{Command: &pb.Command{CommandType: &pb.Command_StartHandshake{StartHandshake: &pb.StartHandshakeCommand{}}}}})
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, startHandshakeCmd))

			_, startHandshakeCmd, err = conn.ReadMessage()
			if tc.wantRecvErr != "" {
				require.EqualError(t, err, "websocket: close 4003: unauthorized")
			} else {
				require.NoError(t, err)

				var msg pb.Envelope
				require.NoError(t, proto.Unmarshal(startHandshakeCmd, &msg))
				require.NotNil(t, msg.GetEvent())
				require.NotNil(t, msg.GetEvent().GetHandshakeCompleted())
			}
		})
	}
}

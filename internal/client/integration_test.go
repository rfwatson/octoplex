//go:build integration

package client_test

import (
	"cmp"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
	containermocks "git.netflux.io/rob/octoplex/internal/generated/mocks/container"
	"git.netflux.io/rob/octoplex/internal/server"
	"git.netflux.io/rob/octoplex/internal/shortid"
	"git.netflux.io/rob/octoplex/internal/store"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const waitTime = time.Minute

func TestIntegration(t *testing.T) {
	t.Run("RTMP with default stream key", func(t *testing.T) {
		testIntegration(t, config.MediaServerSource{
			RTMP: config.RTMPSource{Enabled: true},
		})
	})

	t.Run("RTMPS with default stream key", func(t *testing.T) {
		testIntegration(t, config.MediaServerSource{
			RTMPS: config.RTMPSource{Enabled: true},
		})
	})

	t.Run("RTMP with custom stream key", func(t *testing.T) {
		testIntegration(t, config.MediaServerSource{
			StreamKey: "s0meK3y",
			RTMP: config.RTMPSource{
				Enabled: true,
				NetAddr: config.NetAddr{IP: "0.0.0.0", Port: 3000},
			},
		})
	})

	t.Run("RTMPS with custom stream key", func(t *testing.T) {
		testIntegration(t, config.MediaServerSource{
			StreamKey: "an0therK3y",
			RTMPS: config.RTMPSource{
				Enabled: true,
				NetAddr: config.NetAddr{IP: "0.0.0.0", Port: 443},
			},
		})
	})
}

// hostIP is the IP address of the Docker host from within the container.
//
// This probably only works for Linux.
// https://stackoverflow.com/a/60740997/62871
const hostIP = "172.17.0.1"

func testIntegration(t *testing.T, mediaServerConfig config.MediaServerSource) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	var rtmpConfig config.RTMPSource
	var proto string
	var defaultPort int
	if mediaServerConfig.RTMP.Enabled {
		rtmpConfig = mediaServerConfig.RTMP
		proto = "rtmp://"
		defaultPort = 1935
	} else {
		rtmpConfig = mediaServerConfig.RTMPS
		proto = "rtmps://"
		defaultPort = 1936
	}

	wantRTMPPort := cmp.Or(rtmpConfig.Port, defaultPort)
	wantStreamKey := cmp.Or(mediaServerConfig.StreamKey, "live")
	wantRTMPURL := fmt.Sprintf("%slocalhost:%d/%s", proto, wantRTMPPort, wantStreamKey)

	destServer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "bluenviron/mediamtx:latest",
			ExposedPorts: []string{"1935/tcp"},
			WaitingFor:   wait.ForListeningPort("1935/tcp"),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, destServer)
	require.NoError(t, err)
	destServerPort, err := destServer.MappedPort(ctx, "1935/tcp")
	require.NoError(t, err)

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	destURL1 := fmt.Sprintf("rtmp://%s:%d/live/dest1", hostIP, destServerPort.Int())
	destURL2 := fmt.Sprintf("rtmp://%s:%d/live/dest2", hostIP, destServerPort.Int())

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: mediaServerConfig}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withPersistentState(store.State{
			// Load one destination from config, add the other in-app.
			Destinations: []store.Destination{
				{
					ID:   uuid.MustParse("2e4c97cc-652d-4f3c-b2ee-9594e1c77a3d"),
					Name: "Local server 1",
					URL:  destURL1,
				},
			},
		}),
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   waiting for stream", "expected mediaserver status to be waiting")
		},
		waitTime,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(t, getContents, "After starting the mediaserver")

	// Start streaming a test video to the app:
	testhelpers.StreamFLV(t, wantRTMPURL)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(c, contents[2], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(c, contents[3], "Health   healthy", "expected mediaserver to be healthy")
		},
		waitTime,
		time.Second,
		"expected to receive an ingress stream",
	)
	printScreen(t, getContents, "After receiving the ingress stream")

	// Add a second destination in-app:
	sendKey(t, screen, tcell.KeyRune, 'a')

	sendBackspaces(t, screen, 10)
	sendKeys(t, screen, "Local server 2")
	sendKey(t, screen, tcell.KeyTab, ' ')

	sendBackspaces(t, screen, 10)
	sendKeys(t, screen, destURL2)
	sendKey(t, screen, tcell.KeyTab, ' ')
	sendKey(t, screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(c, contents[2], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(c, contents[3], "Health   healthy", "expected mediaserver to be healthy")

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[3], "off-air", "expected local server 0 to be off-air")

			require.Contains(c, contents[3], "Local server 2", "expected local server 2 to be present")
			assert.Contains(c, contents[3], "off-air", "expected local server 2 to be off-air")

		},
		waitTime,
		time.Second,
		"expected to add the destinations",
	)
	printScreen(t, getContents, "After adding the destinations")

	// Start destinations:
	sendKey(t, screen, tcell.KeyRune, ' ')
	sendKey(t, screen, tcell.KeyDown, ' ')
	sendKey(t, screen, tcell.KeyRune, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(c, contents[2], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(c, contents[3], "Health   healthy", "expected mediaserver to be healthy")

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[2], "sending", "expected local server 1 to be sending")
			assert.Contains(c, contents[2], "healthy", "expected local server 1 to be healthy")

			require.Contains(c, contents[3], "Local server 2", "expected local server 2 to be present")
			assert.Contains(c, contents[3], "sending", "expected local server 2 to be sending")
			assert.Contains(c, contents[3], "healthy", "expected local server 2 to be healthy")
		},
		waitTime,
		time.Second,
		"expected to start the destination streams",
	)
	printScreen(t, getContents, "After starting the destination streams")

	sendKey(t, screen, tcell.KeyUp, ' ')
	sendKey(t, screen, tcell.KeyDelete, ' ')
	sendKey(t, screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(c, contents[2], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(c, contents[3], "Health   healthy", "expected mediaserver to be healthy")

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[2], "sending", "expected local server 1 to be sending")
			assert.Contains(c, contents[2], "healthy", "expected local server 1 to be healthy")

			require.NotContains(c, contents[3], "Local server 2", "expected local server 2 to not be present")

		},
		waitTime,
		time.Second,
		"expected to remove the second destination",
	)
	printScreen(t, getContents, "After removing the second destination")

	// Stop remaining destination.
	sendKey(t, screen, tcell.KeyRune, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[2], "exited", "expected local server 1 to have exited")

			require.NotContains(c, contents[3], "Local server 2", "expected local server 2 to not be present")
		},
		waitTime,
		time.Second,
		"expected to stop the first destination stream",
	)

	printScreen(t, getContents, "After stopping the first destination")

	cancel()

	result := <-ch
	// May be a gRPC error, not context.Canceled:
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationCustomHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{
			Host: "rtmp.example.com",
			Sources: config.Sources{
				MediaServer: config.MediaServerSource{
					RTMP: config.RTMPSource{Enabled: true},
				},
			},
		},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
	)
	ch := runClientServer(ctx, t, client, server)

	time.Sleep(time.Second)
	sendKey(t, screen, tcell.KeyF1, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			assert.True(t, contentsIncludes(getContents(), "rtmp://rtmp.example.com:1935/live"), "expected to see custom host name")
		},
		waitTime,
		time.Second,
		"expected to see custom host name",
	)
	printScreen(t, getContents, "Ater opening the app with a custom host name")

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			conn, err := tls.Dial("tcp", "localhost:9997", &tls.Config{InsecureSkipVerify: true})
			require.NoError(c, err)

			require.Nil(
				c,
				conn.
					ConnectionState().
					PeerCertificates[0].
					VerifyHostname("rtmp.example.com"),
				"expected to verify custom host name",
			)
		},
		waitTime,
		time.Second,
		"expected to connect to API using self-signed TLS cert with custom host name",
	)

	cancel()

	result := <-ch
	// May be a gRPC error, not context.Canceled:
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationTLSCerts(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, _ := setupSimulationScreen(t)

	dataDir := filepath.Join(os.TempDir(), "octoplex-test-data-"+shortid.New().String())
	require.NoError(t, os.MkdirAll(dataDir, 0700))
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	client, srv := buildClientServer(
		t,
		config.Config{
			DataDir: dataDir,
			Sources: config.Sources{MediaServer: config.MediaServerSource{RTMPS: config.RTMPSource{Enabled: true}}},
		},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withListenerFunc(server.WithListener(lis)),
	)
	ch := runClientServer(ctx, t, client, srv)

	var certs [][]byte
	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			certs = nil
			rtmpPort := "localhost:1936"
			grpcPort := fmt.Sprintf("localhost:%d", lis.Addr().(*net.TCPAddr).Port)
			for _, addr := range []string{rtmpPort, grpcPort} {
				conn, dialErr := tls.Dial("tcp", addr, &tls.Config{ServerName: "localhost", InsecureSkipVerify: true})
				require.NoErrorf(c, dialErr, "failed to connect to %s: %v", addr, dialErr)
				defer conn.Close()

				peerCerts := conn.ConnectionState().PeerCertificates
				require.Len(c, peerCerts, 1, "expected one peer certificate for %s", addr)
				certBytes := sha256.Sum256(peerCerts[0].Raw)
				certs = append(certs, certBytes[:])
			}
		},
		waitTime,
		time.Second,
	)

	cancel()
	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)

	// now, restart the server and ensure the certs are the same

	ctx, cancel = context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	lis, err = net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	client, srv = buildClientServer(
		t,
		config.Config{
			DataDir: dataDir,
			Sources: config.Sources{MediaServer: config.MediaServerSource{RTMPS: config.RTMPSource{Enabled: true}}},
		},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withListenerFunc(server.WithListener(lis)),
	)
	ch = runClientServer(ctx, t, client, srv)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			rtmpPort := "localhost:1936"
			grpcPort := fmt.Sprintf("localhost:%d", lis.Addr().(*net.TCPAddr).Port)
			for i, addr := range []string{rtmpPort, grpcPort} {
				conn, dialErr := tls.Dial("tcp", addr, &tls.Config{ServerName: "localhost", InsecureSkipVerify: true})
				require.NoErrorf(c, dialErr, "failed to connect to %s: %v", addr, dialErr)
				defer conn.Close()

				peerCert := conn.ConnectionState().PeerCertificates[0]
				certBytes := sha256.Sum256(peerCert.Raw)
				assert.Equal(c, certs[i], certBytes[:], "expected cert for %s to be the same as before", addr)
			}
		},
		waitTime,
		time.Second,
	)

	cancel()
	result = <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationCustomTLSCerts(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	client, server := buildClientServer(
		t,
		config.Config{
			TLS: &config.TLS{
				CertPath: "testdata/server.crt",
				KeyPath:  "testdata/server.key",
			},
			Sources: config.Sources{
				MediaServer: config.MediaServerSource{
					RTMPS: config.RTMPSource{Enabled: true},
				},
			},
		},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withListenerFunc(server.WithListener(lis)),
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			certPEM, err := os.ReadFile("testdata/server.crt")
			require.NoError(c, err)

			block, _ := pem.Decode(certPEM)
			require.NotNil(c, block, "failed to decode PEM block containing certificate")
			require.True(c, block.Type == "CERTIFICATE", "expected PEM block to be a certificate")

			rootCAs := x509.NewCertPool()
			require.True(c, rootCAs.AppendCertsFromPEM(certPEM), "failed to append cert to root CA pool")

			rtmpPort := "localhost:1936"
			grpcPort := fmt.Sprintf("localhost:%d", lis.Addr().(*net.TCPAddr).Port)
			for _, addr := range []string{rtmpPort, grpcPort} {
				conn, err := tls.Dial("tcp", addr, &tls.Config{
					RootCAs:            rootCAs,
					ServerName:         "localhost",
					InsecureSkipVerify: false,
				})
				require.NoErrorf(c, err, "failed to connect to %s: %v", addr, err)
				defer conn.Close()

				peerCert := conn.ConnectionState().PeerCertificates[0]
				wantCert, err := x509.ParseCertificate(block.Bytes)
				require.NoError(c, err)
				require.Truef(c, peerCert.Equal(wantCert), "expected peer cert for host %s to match", addr)
			}
		},
		waitTime,
		time.Second,
	)

	printScreen(t, getContents, "After starting the app with custom TLS certs")

	cancel()

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

// TODO: pass TLS certs from client and test that too
func TestIntegrationCustomTLSNoInsecureSkipVerify(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, _ := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{
			TLS: &config.TLS{
				CertPath: "testdata/server.crt",
				KeyPath:  "testdata/server.key",
			},
			Sources: config.Sources{MediaServer: config.MediaServerSource{RTMPS: config.RTMPSource{Enabled: true}}},
		},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withInsecureSkipVerify(false),
	)

	result := <-runClientServer(ctx, t, client, server)
	assert.ErrorContains(t, result.errClient, "failed to verify certificate: x509: certificate signed by unknown authority")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationRestartDestination(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	destServer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "bluenviron/mediamtx:latest",
			Env:          map[string]string{"MTX_LOGLEVEL": "debug"},
			ExposedPorts: []string{"1936/tcp", "9997/tcp"},
			WaitingFor:   wait.ForListeningPort("1936/tcp"),
		},
		Started: false,
	})
	testcontainers.CleanupContainer(t, destServer)
	require.NoError(t, err)

	require.NoError(t, destServer.CopyFileToContainer(t.Context(), "testdata/mediamtx.yml", "/mediamtx.yml", 0600))
	require.NoError(t, destServer.Start(ctx))

	destServerRTMPPort, err := destServer.MappedPort(ctx, "1936/tcp")
	require.NoError(t, err)

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}}}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withPersistentState(store.State{
			Destinations: []store.Destination{
				{
					ID:   uuid.MustParse("dd100899-5801-423d-8c1f-569edfde10dd"),
					Name: "Local server 1",
					URL:  fmt.Sprintf("rtmp://%s:%d/live", hostIP, destServerRTMPPort.Int()),
				},
			},
		}),
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   waiting for stream", "expected mediaserver status to be waiting")
		},
		waitTime,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(t, getContents, "After starting the mediaserver")

	// Start streaming a test video to the app:
	testhelpers.StreamFLV(t, "rtmp://localhost:1935/live")

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")
		},
		waitTime,
		time.Second,
		"expected to receive an ingress stream",
	)
	printScreen(t, getContents, "After receiving the ingress stream")

	// Start destination:
	sendKey(t, screen, tcell.KeyRune, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[2], "sending", "expected local server 1 to be sending")
			assert.Contains(c, contents[2], "healthy", "expected local server 1 to be healthy")
		},
		waitTime,
		time.Second,
		"expected to start the destination stream",
	)
	printScreen(t, getContents, "After starting the destination stream")

	// Wait for enough time that the container will be restarted.
	// Then, kick the connection to force a restart.
	time.Sleep(15 * time.Second)
	kickFirstRTMPConn(t, destServer)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[2], "off-air", "expected local server 1 to be off-air")
			assert.Contains(c, contents[2], "restarting", "expected local server 1 to be restarting")
		},
		waitTime,
		time.Second,
		"expected to begin restarting",
	)
	printScreen(t, getContents, "After stopping the destination server")

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[2], "sending", "expected local server 1 to be sending")
			assert.Contains(c, contents[2], "healthy", "expected local server 1 to be healthy")
		},
		waitTime,
		time.Second,
		"expected to restart the destination stream",
	)
	printScreen(t, getContents, "After restarting the destination stream")

	// Stop destination.
	sendKey(t, screen, tcell.KeyRune, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			require.Contains(c, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(c, contents[2], "exited", "expected local server 1 to have exited")

			require.NotContains(c, contents[3], "Local server 2", "expected local server 2 to not be present")
		},
		waitTime,
		time.Second,
		"expected to stop the destination stream",
	)

	printScreen(t, getContents, "After stopping the destination")

	cancel()

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationUpdateDestination(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}}}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withPersistentState(store.State{
			Destinations: []store.Destination{
				{
					ID:   uuid.MustParse("ca390fbe-1028-46ad-be5c-2d70f81d2eec"),
					Name: "Stream name 1",
					URL:  "rtmp://rtmp.example.com/live",
				}},
		}),
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   waiting for stream", "expected mediaserver status to be waiting")
		},
		waitTime,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(t, getContents, "After starting the mediaserver")

	// Edit destination:
	sendKey(t, screen, tcell.KeyRune, 'e')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()
			assert.True(c, contentsIncludes(contents, "Save changes"), "expected to see update destination form")
		},
		waitTime,
		time.Second,
		"expected to see the update destination form",
	)
	printScreen(t, getContents, "After opening the update destination form")

	sendBackspaces(t, screen, 13)
	sendKeys(t, screen, "New name")
	sendKey(t, screen, tcell.KeyEnter, ' ')
	sendKey(t, screen, tcell.KeyRune, '2')
	sendKey(t, screen, tcell.KeyEnter, ' ')
	sendKey(t, screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.False(c, contentsIncludes(contents, "Save changes"), "expected not to see update destination form")
			assert.True(c, contentsIncludes(contents, "New name           rtmp://rtmp.example.com/live2           off-air"), "expected to see updated destination")
		},
		waitTime,
		time.Second,
		"expected the destination to be updated",
	)
	printScreen(t, getContents, "After updating the destination")

	cancel()

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationStartDestinationFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}}}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
		withPersistentState(store.State{
			Destinations: []store.Destination{
				{
					ID:   uuid.MustParse("85d1e543-efda-4440-932f-69ff49c21f2e"),
					Name: "Example server",
					URL:  "rtmp://rtmp.example.com/live",
				}},
		}),
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   waiting for stream", "expected mediaserver status to be waiting")
		},
		waitTime,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(t, getContents, "After starting the mediaserver")

	// Start streaming a test video to the app:
	testhelpers.StreamFLV(t, "rtmp://localhost:1935/live")

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.Contains(c, contents[1], "Status   receiving", "expected mediaserver status to be receiving")
		},
		waitTime,
		time.Second,
		"expected to receive an ingress stream",
	)
	printScreen(t, getContents, "After receiving the ingress stream")

	// Start destination:
	sendKey(t, screen, tcell.KeyRune, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()
			assert.True(c, contentsIncludes(contents, "Streaming to Example server failed:"), "expected to see destination error")
			assert.True(c, contentsIncludes(contents, "Error opening output files: I/O error"), "expected to see destination error")
		},
		waitTime,
		time.Second,
		"expected to see the destination start error modal",
	)
	printScreen(t, getContents, "After starting the destination stream fails")

	cancel()

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationDestinationValidations(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{StreamKey: "live", RTMP: config.RTMPSource{Enabled: true}}}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()
			require.True(c, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(c, contents[1], "Status   waiting for stream", "expected mediaserver status to be waiting")
			assert.True(c, contentsIncludes(contents, "No destinations added yet. Press [a] to add a new destination."), "expected to see no destinations message")
		},
		waitTime,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(t, getContents, "After starting the mediaserver")

	sendKey(t, screen, tcell.KeyRune, 'a')
	sendKey(t, screen, tcell.KeyTab, ' ')
	sendBackspaces(t, screen, 10)
	sendKey(t, screen, tcell.KeyTab, ' ')
	sendKey(t, screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.True(c, contentsIncludes(contents, "Configuration update failed:"), "expected to see config update error")
			assert.True(c, contentsIncludes(contents, "validate: destination URL must be an RTMP URL"), "expected to see invalid RTMP URL error")
		},
		waitTime,
		time.Second,
		"expected a validation error for an empty URL",
	)
	printScreen(t, getContents, "After entering an empty destination URL")

	sendKey(t, screen, tcell.KeyEnter, ' ')
	sendKey(t, screen, tcell.KeyBacktab, ' ')
	sendKeys(t, screen, "nope")
	sendKey(t, screen, tcell.KeyTab, ' ')
	sendKey(t, screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.True(c, contentsIncludes(contents, "Configuration update failed:"), "expected to see config update error")
			assert.True(c, contentsIncludes(contents, "validate: destination URL must be an RTMP URL"), "expected to see invalid RTMP URL error")
		},
		waitTime,
		time.Second,
		"expected a validation error for an invalid URL",
	)
	printScreen(t, getContents, "After entering an invalid destination URL")

	sendKey(t, screen, tcell.KeyEnter, ' ')
	sendKey(t, screen, tcell.KeyBacktab, ' ')
	sendBackspaces(t, screen, len("nope"))
	sendKeys(t, screen, "rtmp://rtmp.youtube.com:1935/live")
	sendKey(t, screen, tcell.KeyTab, ' ')
	sendKey(t, screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			require.Contains(c, contents[2], "My stream", "expected new destination to be present")
			assert.Contains(c, contents[2], "off-air", "expected new destination to be off-air")
			assert.False(c, contentsIncludes(contents, "No destinations added yet"), "expected to not see no destinations message")
		},
		waitTime,
		time.Second,
		"expected to add the destination",
	)
	printScreen(t, getContents, "After adding the destination")

	sendKey(t, screen, tcell.KeyRune, 'a')
	sendKey(t, screen, tcell.KeyTab, ' ')
	sendBackspaces(t, screen, 10)
	sendKeys(t, screen, "rtmp://rtmp.youtube.com:1935/live")
	sendKey(t, screen, tcell.KeyTab, ' ')
	sendKey(t, screen, tcell.KeyEnter, ' ')

	// Start streaming a test video to the app:
	testhelpers.StreamFLV(t, "rtmp://localhost:1935/live")

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()

			assert.True(c, contentsIncludes(contents, "Configuration update failed:"), "expected to see config update error")
			assert.True(c, contentsIncludes(contents, "validate: duplicate destination URL: rtmp://"), "expected to see config update error")
		},
		waitTime,
		time.Second,
		"expected a validation error for a duplicate URL",
	)
	printScreen(t, getContents, "After entering a duplicate destination URL")
	cancel()

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationStartupCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	// Start a container that looks like a stale Octoplex container:
	staleContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "bluenviron/mediamtx:latest",
			Env:          map[string]string{"MTX_RTMPADDRESS": ":1937"},
			ExposedPorts: []string{"1937/tcp"},
			WaitingFor:   wait.ForListeningPort("1937/tcp"),
			// It doesn't matter what image this container uses, as long as it has
			// labels that look like an Octoplex container:
			Labels: map[string]string{container.LabelApp: domain.AppName},
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, staleContainer)
	require.NoError(t, err)

	require.NoError(t, err)
	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}}}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			assert.True(c, contentsIncludes(getContents(), "Another instance of Octoplex may already be running."), "expected to see startup check modal")
		},
		waitTime,
		time.Second,
		"expected to see startup check modal",
	)
	printScreen(t, getContents, "Ater displaying the startup check modal")

	sendKey(t, screen, tcell.KeyEnter, ' ') // quit other containers

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			_, err := staleContainer.State(context.Background())
			// IsRunning() does not work, probably because we're undercutting the
			// testcontainers API.
			require.True(c, errdefs.IsNotFound(err), "expected to not find the container")
		},
		waitTime,
		2*time.Second,
		"expected to quit the other containers",
	)
	printScreen(t, getContents, "After quitting the other containers")

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			contents := getContents()
			require.True(c, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(c, contents[1], "Status   waiting for stream", "expected mediaserver status to be waiting")
		},
		waitTime,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(t, getContents, "After starting the mediaserver")
	cancel()

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorIs(t, result.errServer, context.Canceled)
}

func TestIntegrationMediaServerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	lis, err := net.Listen("tcp", ":1935")
	require.NoError(t, err)
	t.Cleanup(func() { lis.Close() })

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}}}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			assert.True(c, contentsIncludes(getContents(), "Server process exited unexpectedly."), "expected to see title")
			assert.True(c, contentsIncludes(getContents(), "address already in use"), "expected to see message")
		},
		waitTime,
		time.Second,
		"expected to see media server error modal",
	)
	printScreen(t, getContents, "Ater displaying the media server error modal")

	// Quit the app:
	sendKey(t, screen, tcell.KeyEnter, ' ')

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorContains(t, result.errServer, "media server exited")
}

func TestIntegrationDockerClientError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")

	var dockerClient containermocks.DockerClient
	dockerClient.EXPECT().NetworkCreate(mock.Anything, mock.Anything, mock.Anything).Return(network.CreateResponse{}, errors.New("boom"))

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}}}},
		&dockerClient,
		screen,
		screenCaptureC,
		logger,
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			assert.True(c, contentsIncludes(getContents(), "An error occurred:"), "expected to see error message")
			assert.True(c, contentsIncludes(getContents(), "create container client: network create: boom"), "expected to see message")
		},
		waitTime,
		time.Second,
		"expected to see fatal error modal",
	)
	printScreen(t, getContents, "Ater displaying the fatal error modal")

	// Quit the app:
	sendKey(t, screen, tcell.KeyEnter, ' ')

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.EqualError(t, result.errServer, "create container client: network create: boom")
}

func TestIntegrationDockerConnectionError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("http://docker.example.com"))
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	client, server := buildClientServer(
		t,
		config.Config{Sources: config.Sources{MediaServer: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}}}},
		dockerClient,
		screen,
		screenCaptureC,
		logger,
	)
	ch := runClientServer(ctx, t, client, server)

	require.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			assert.True(c, contentsIncludes(getContents(), "An error occurred:"), "expected to see error message")
			assert.True(c, contentsIncludes(getContents(), "Could not connect to Docker. Is Docker installed"), "expected to see message")
		},
		waitTime,
		time.Second,
		"expected to see fatal error modal",
	)
	printScreen(t, getContents, "Ater displaying the fatal error modal")

	// Quit the app:
	sendKey(t, screen, tcell.KeyEnter, ' ')

	result := <-ch
	assert.ErrorContains(t, result.errClient, "context canceled")
	assert.ErrorContains(t, result.errServer, "dial tcp: lookup docker.example.com")
	assert.ErrorContains(t, result.errServer, "no such host")
}

func TestIntegrationCopyURLs(t *testing.T) {
	type binding struct {
		key     tcell.Key
		content string
		url     string
	}

	testCases := []struct {
		name              string
		mediaServerConfig config.MediaServerSource
		wantBindings      []binding
		wantNot           []string
	}{
		{
			name:              "RTMP only",
			mediaServerConfig: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}},
			wantBindings: []binding{
				{
					key:     tcell.KeyF1,
					content: "F1       Copy source RTMP URL",
					url:     "rtmp://localhost:1935/live",
				},
			},
			wantNot: []string{"F2"},
		},
		{
			name:              "RTMPS only",
			mediaServerConfig: config.MediaServerSource{RTMPS: config.RTMPSource{Enabled: true}},
			wantBindings: []binding{
				{
					key:     tcell.KeyF1,
					content: "F1       Copy source RTMPS URL",
					url:     "rtmps://localhost:1936/live",
				},
			},
			wantNot: []string{"F2"},
		},
		{
			name:              "RTMP and RTMPS",
			mediaServerConfig: config.MediaServerSource{RTMP: config.RTMPSource{Enabled: true}, RTMPS: config.RTMPSource{Enabled: true}},
			wantBindings: []binding{
				{
					key:     tcell.KeyF1,
					content: "F1       Copy source RTMP URL",
					url:     "rtmp://localhost:1935/live",
				},
				{
					key:     tcell.KeyF2,
					content: "F2       Copy source RTMPS URL",
					url:     "rtmps://localhost:1936/live",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()

			logger := testhelpers.NewTestLogger(t).With("component", "integration")
			dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
			require.NoError(t, err)

			screen, screenCaptureC, getContents := setupSimulationScreen(t)

			client, server := buildClientServer(
				t,
				config.Config{Sources: config.Sources{MediaServer: tc.mediaServerConfig}},
				dockerClient,
				screen,
				screenCaptureC,
				logger,
			)
			ch := runClientServer(ctx, t, client, server)

			for _, want := range tc.wantBindings {
				require.EventuallyWithT(
					t,
					func(c *assert.CollectT) {
						assert.True(c, contentsIncludes(getContents(), want.content), "expected to see %q", want)
					},
					3*time.Second,
					100*time.Millisecond,
				)

				sendKey(t, screen, want.key, ' ')

				require.EventuallyWithT(
					t,
					func(c *assert.CollectT) {
						assert.True(c, contentsIncludes(getContents(), want.url), "expected to see copied message")
					},
					3*time.Second,
					100*time.Millisecond,
				)

				printScreen(t, getContents, "After pressing the copy key")

				sendKey(t, screen, tcell.KeyEscape, ' ')
			}

			for _, wantNot := range tc.wantNot {
				assert.False(t, contentsIncludes(getContents(), wantNot), "expected to not see %q", wantNot)
			}

			cancel()

			result := <-ch
			assert.ErrorContains(t, result.errClient, "context canceled")
			assert.ErrorIs(t, result.errServer, context.Canceled)
		})
	}
}

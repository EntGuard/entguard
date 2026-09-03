/*
 * Copyright 2020 PANTHEON.tech s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vpns

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"

	"github.com/entguard/entguard/cmd/vpn_server/vpns/wireguard"
	"github.com/entguard/entguard/pkg/limiter"
	config "github.com/entguard/entguard/pkg/server-config"
	pbapi "github.com/entguard/entguard/proto/v1"
)

const (
	// Unix socket buffer size in bytes
	bufferSize       = 20 * 1024
	socketPath       = "/run/sockets/socket"
	sleepTimeOnError = 1 * time.Second
	// Should be greater than Telegraf flush interval (default: 5s) AND greater
	// than Telegraf initial connection retry interval (default: 15s)
	listenerTimeout = 17 * time.Second
	// Should be long enough to stop the telemetry, but not too long so it
	// leaves some time for the rest of shutdown procedure before docker kills
	// the container
	telemetryStopTimeout = 7 * time.Second
)

// VpnS is a VPN server configurator.
type VpnS struct {
	cfg      *config.Config
	log      *log.Entry
	conn     *grpc.ClientConn
	wgc      wireguard.Client
	tlsCfg   *tls.Config
	vpnCfg   *pbapi.Configuration
	stop     context.CancelFunc
	connErrs chan error

	// If this field is set and the telemetry is an enabled feature in EG-O,
	// there will be an attempt to create a unix socket for Telegraf metrics.
	enableTelemetry  bool
	stopTelemetry    context.CancelFunc
	telemetryStopped chan struct{}
}

// NewVpnS returns a new VpnS with given configuration.
func NewVpnS(cfg *config.Config, tlsCfg *tls.Config, enableTelemetry bool, logger *log.Entry) *VpnS {
	return &VpnS{
		cfg:              cfg,
		log:              logger,
		connErrs:         make(chan error),
		enableTelemetry:  enableTelemetry,
		telemetryStopped: make(chan struct{}),
		tlsCfg:           tlsCfg,
	}
}

// Connect connects VpnS with VpnO.
func (vpns *VpnS) Connect(initial bool) error {
	var err error

	vpns.log.WithField("TargetRPCAddress", vpns.cfg.RPCAddr).
		Info("Connecting to EntGuard orchestrator")
	vpns.conn, err = vpns.createConnection()
	if err != nil {
		return fmt.Errorf("could not connect to the EntGuard orchestrator: %v", err)
	}

	// activate connection
	vpns.conn.Connect()
	time.Sleep(1 * time.Second)
	vpns.log.Info("State of RPC connection with Orchestrator: ", vpns.conn.GetState().String())

	vpns.log.Info("Requesting configuration")
	rpcClient := pbapi.NewVpnSClient(vpns.conn)
	vpnCfg, err := vpns.requestConfiguration(rpcClient)
	if err != nil {
		return fmt.Errorf("could not retrieve configuration: %v", err)
	}

	if initial {
		vpns.log.Info("Configuring initial setup")
		vpns.vpnCfg = vpnCfg
		if err := vpns.configureVPN(vpns.vpnCfg); err != nil {
			return fmt.Errorf("could not setup VPN: %v", err)
		}
	} else {
		vpns.log.Info("Reconfiguring setup")
		if err = vpns.reconfigureVPN(vpnCfg); err != nil {
			return fmt.Errorf("could not reconfigure VPN: %v", err)
		}
	}
	vpns.log.Info("WireGuard configuration successfully applied")

	if vpns.vpnCfg.TelemetryEnabled && vpns.enableTelemetry {
		vpns.log.Info("Starting telemetry")
		ctx, cancel := context.WithCancel(context.Background())
		err := vpns.startSendingTelemetry(ctx, rpcClient)
		vpns.stopTelemetry = cancel
		if err != nil {
			return fmt.Errorf("could not start sending telemetry: %v", err)
		}
		vpns.log.Info("Telemetry started")
	}

	vpns.log.Info("Subscribing for configuration updates")
	err = vpns.subscribeForChanges(vpns.vpnCfg, vpns.cfg, rpcClient)
	if err != nil {
		return fmt.Errorf("could not subscribe for configuration changes: %v", err)
	}
	vpns.log.Info("Subscribed for configuration updates")

	return nil
}

// Disconnect disconnects VpnS from VpnO.
func (vpns *VpnS) Disconnect(removeWG bool) {
	if vpns.stop != nil {
		vpns.log.Info("Stopping processing updates")
		vpns.stop()
		vpns.stop = nil
	}
	if vpns.stopTelemetry != nil {
		vpns.log.Info("Stopping telemetry")
		vpns.stopTelemetry()

		select {
		case <-vpns.telemetryStopped:
			vpns.log.Debug("Telemetry stopped")
		case <-time.After(telemetryStopTimeout):
			vpns.log.Warn("Stopping telemetry timed out")
		}

		vpns.stopTelemetry = nil
	}
	if vpns.conn != nil {
		vpns.log.Info("Disconnecting from EntGuard orchestrator")
		if err := vpns.conn.Close(); err != nil {
			vpns.log.WithError(err).Error("Closing gRPC connection")
		}
		vpns.conn = nil
	}
	if vpns.wgc != nil && removeWG {
		vpns.log.Info("Removing WireGuard VPN setup")
		vpns.wgc.Close()
	}
}

func (vpns *VpnS) ReconnectOnError() {
	for streamUpdatesErr := range vpns.connErrs {
		vpns.log.WithError(streamUpdatesErr).
			Warn("Reconnecting because an error occured")

		var err error

		for i := 1; i <= vpns.cfg.ReconnectAttempts; i++ {
			if i != 1 {
				vpns.log.WithField("reconnectPause", vpns.cfg.ReconnectPause).
					Debug("Sleeping before next reconnect attempt")
				time.Sleep(vpns.cfg.ReconnectPause)
			}

			vpns.log.WithField("reconnectAttempt", i).
				Info("Reconnecting...")
			err = vpns.reconnectAttempt()
			if err != nil {
				vpns.log.WithError(err).
					Info("Reconnect attempt failed")
				continue
			}
			break
		}
		if err != nil {
			vpns.log.Error("All reconnecting attempts failed.")

			vpns.log.Info("Shutting down.")
			vpns.Disconnect(true)
			os.Exit(1)
		}
	}
}

func (vpns *VpnS) reconnectAttempt() error {
	// Releasing resources
	vpns.Disconnect(false)

	// Trying to reconnect
	return vpns.Connect(false)
}

// createConnection returns gRPC client connection.
func (vpns *VpnS) createConnection() (*grpc.ClientConn, error) {
	creds := credentials.NewTLS(vpns.tlsCfg)
	return grpc.NewClient(vpns.cfg.RPCAddr, grpc.WithTransportCredentials(creds))
}

func (vpns *VpnS) requestConfiguration(client pbapi.VpnSClient) (*pbapi.Configuration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), vpns.cfg.RPCTimeout)
	defer cancel()
	vpnCfg, err := client.GetConfiguration(ctx, &pbapi.Empty{})
	if err != nil {
		return nil, fmt.Errorf("request for VPN configuration failed: %v", err)
	}
	if vpnCfg == nil {
		return nil, errors.New("received nil as configuration")
	}
	return vpnCfg, nil
}

func (vpns *VpnS) configureVPN(vpnCfg *pbapi.Configuration) error {
	switch vpnCfg.Type {
	case pbapi.Configuration_UNDEFINED_TYPE:
		return errors.New("received configuration of undefined type")

	case pbapi.Configuration_WIREGUARD:
		vpns.log.Debug("Received VPN configuration for WireGuard.")
		if vpnCfg.GetWg() == nil {
			return errors.New("received configuration type does not match its content")
		}
		err := vpns.configureWireGuardVPN(vpnCfg.GetWg())
		if err != nil {
			return err
		}
		return nil

	default:
		return errors.New("received configuation of unknown/unsupported type")
	}
}

func (vpns *VpnS) reconfigureVPN(vpnCfg *pbapi.Configuration) error {
	if proto.Equal(vpns.vpnCfg, vpnCfg) {
		// nothing has changed
		vpns.log.Debug("Current configuration matches with received.")
		return nil
	}
	vpns.log.Debug("VPN configuration has changed while establishing connection.")
	if vpns.vpnCfg.Type != vpnCfg.Type {
		return errors.New("type of VPN configuration has changed")
	}

	switch vpnCfg.Type {
	case pbapi.Configuration_WIREGUARD:
		vpns.log.Debug("Received VPN configuration for WireGuard.")
		if vpnCfg.GetWg() == nil {
			return errors.New("received configuration type does not match its content")
		}
		err := vpns.reconfigureWireGuardVPN(vpnCfg.GetWg())
		if err != nil {
			return fmt.Errorf("failed to reconfigure WireGuard: %v", err)
		}
		return nil

	default:
		return errors.New("received configuation of unknown/unsupported type")
	}
}

func (vpns *VpnS) subscribeForChanges(vpnCfg *pbapi.Configuration, vpnsConfig *config.Config, client pbapi.VpnSClient) error {
	switch vpnCfg.Type {
	case pbapi.Configuration_WIREGUARD:
		vpns.log.Debug("Subscribing for WireGuard updates.")

		ctx, cancel := context.WithCancel(context.Background())
		vpns.stop = cancel
		go vpns.processWireGuardUpdates(ctx, client, vpnsConfig)

		return nil

	default:
		return errors.New("cannot subscribe for configuration changes for unknown/unsupported VPN type")
	}
}

func (vpns *VpnS) configureWireGuardVPN(wgCfg *pbapi.WireGuard) error {
	wgc, err := wireguard.NewClient(vpns.log, vpns.cfg.UseVPP)
	if err != nil {
		return err
	}
	vpns.wgc = wgc

	vpns.log.Info("wg configuration: creating wg interface and peers")
	err = vpns.wgc.AddWireGuard(wgCfg)
	if err != nil {
		return fmt.Errorf("could not configure WireGuard based VPN: %v", err)
	}

	return nil
}

// reconfigureWireGuardVPN configures WireGuard VPN according to vpnCfg but with knowledge that the VPN was
// already configured previously (i.e. reconfiguration after reconnection to orchestrator after temporal
// connection failure)
func (vpns *VpnS) reconfigureWireGuardVPN(wgNew *pbapi.WireGuard) error {
	if vpns.wgc == nil {
		return errors.New("missing WireGuard client")
	}
	if vpns.vpnCfg.GetWg() == nil {
		return errors.New("missing old WireGuard configuration")
	}

	// Updating wireguard interface and its attributes (without peers)
	var err error
	fromScratchLogMessage := "proceeding to teardown and build wg interfaces and peers from scratch because " +
		"can't do in-place update of %s"
	wgOld := vpns.vpnCfg.GetWg()
	if (wgOld.Name != wgNew.Name) || !proto.Equal(wgOld.Iface, wgNew.Iface) {
		vpns.log.Info("wg reconfiguration: updating wg interface attributes")
		err = vpns.wgc.UpdateWireGuardInterface(wgNew)
		if err != nil {
			vpns.log.WithError(err).
				Warnf(fromScratchLogMessage, "WG interface")
			return vpns.reconfigureWireGuardVPNFromScratch(wgNew)
		}
	}

	// Updating peers:
	// Fast case 1: from 0 to N peers.
	if len(wgOld.Peers) == 0 && len(wgNew.Peers) > 0 {
		vpns.log.Infof("wg reconfiguration: Adding %d peers", len(wgNew.Peers))
		err = vpns.wgc.AddPeers(wgNew.Peers...)
		if err != nil {
			vpns.log.WithError(err).
				Warnf(fromScratchLogMessage, "peers(case1)")
			return vpns.reconfigureWireGuardVPNFromScratch(wgNew)
		}
		vpns.updateExistingWGConfigInternally(wgNew) // when success then update internal configuration
		return nil
	}

	// Fast case 2: from N to 0 peers.
	if len(wgOld.Peers) > 0 && len(wgNew.Peers) == 0 {
		vpns.log.Infof("wg reconfiguration: Removing %d peers", len(wgOld.Peers))
		err = vpns.wgc.DelPeers(wgOld.Peers...)
		if err != nil {
			vpns.log.WithError(err).
				Warnf(fromScratchLogMessage, "peers(case2)")
			return vpns.reconfigureWireGuardVPNFromScratch(wgNew)
		}
		vpns.updateExistingWGConfigInternally(wgNew) // when success then update internal configuration
		return nil
	}

	var (
		toAdd []*pbapi.WireGuardPeer
		toDel []*pbapi.WireGuardPeer
		found bool
	)

	// Building a list of peers which needs to be removed.
	// These peers are present in current configuration, but missing in received one.
	for _, p1 := range wgOld.Peers {
		found = false
		for _, p2 := range wgNew.Peers {
			if proto.Equal(p1, p2) {
				found = true
				break
			}
		}
		if !found {
			toDel = append(toDel, p1)
		}
	}

	// Building a list of peers which needs to be added.
	// These peers are missing in current configuration, but present in received one.
	for _, p2 := range wgNew.Peers {
		found = false
		for _, p1 := range wgOld.Peers {
			if proto.Equal(p1, p2) {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, p2)
		}
	}

	if len(toDel) > 0 {
		vpns.log.Infof("wg reconfiguration: Removing %d peers", len(toDel))
		err = vpns.wgc.DelPeers(toDel...)
		if err != nil {
			vpns.log.WithError(err).
				Warnf(fromScratchLogMessage, "peers(delete of peers)")
			return vpns.reconfigureWireGuardVPNFromScratch(wgNew)
		}
	}
	if len(toAdd) > 0 {
		vpns.log.Infof("wg reconfiguration: Adding %d peers", len(toAdd))
		err = vpns.wgc.AddPeers(toAdd...)
		if err != nil {
			vpns.log.WithError(err).
				Warnf(fromScratchLogMessage, "peers(adding of peers)")
			return vpns.reconfigureWireGuardVPNFromScratch(wgNew)
		}
	}

	// FIXME no configuration application rollback if something fails -> inconsistent vpns.vpnCfg (and possibly
	//  also vpns.wgc.wgCfg and vpns.wgc.wgDev) and setup state (for problems with update the rebuild from
	//  scratch is already applied but that can fail too)

	vpns.updateExistingWGConfigInternally(wgNew) // when success then update internal configuration
	return nil
}

// updateExistingWGConfigInternally update server's internal configuration (while expecting existence of some
// old WG config). This is needed i.e. after successful update of underlay client with updated config
// from notification from orchestrator.
func (vpns *VpnS) updateExistingWGConfigInternally(newWGConfig *pbapi.WireGuard) {
	vpns.vpnCfg.VpnProto.(*pbapi.Configuration_Wg).Wg = newWGConfig
	// Note: for vpns.wgc.wgCfg and vpns.wgc.wgDev are updated inside client (vpns.wgc) as part of the device and
	// interface update process. This is sooner than end of the update (kind of "commit of transaction") and it
	// is valid due to any fail after that point is handled by reconfigureWireGuardVPNFromScratch that throws
	// out client and creates new one with updated configs
}

func (vpns *VpnS) reconfigureWireGuardVPNFromScratch(wgNew *pbapi.WireGuard) error {
	vpns.log.Info("reconfiguration from scratch: removing old wg interface and peers")
	vpns.wgc.Close() // also deletes the WG interface

	vpns.log.Info("reconfiguration from scratch: adding new wg interface")
	err := vpns.configureWireGuardVPN(wgNew)
	if err != nil {
		return fmt.Errorf("can't reconfigure WireGuard VPN from scratch: %w", err)
	}

	vpns.log.Infof("reconfiguration from scratch: adding %d peers", len(wgNew.Peers))
	err = vpns.wgc.AddPeers(wgNew.Peers...)
	if err != nil {
		return fmt.Errorf("can't add peers in reconfiguration from scratch: %w", err)
	}

	vpns.updateExistingWGConfigInternally(wgNew) // when success then update internal configuration
	return nil
}

func updateWGResponse(code pbapi.RPCErrorCode, err error) *pbapi.SubscribeWGRequest {
	var msg string
	if err != nil {
		msg = err.Error()
	}
	return &pbapi.SubscribeWGRequest{
		Data: &pbapi.SubscribeWGRequest_Response{
			Response: &pbapi.RPCStatus{
				Code:    code,
				Message: msg,
			},
		},
	}
}

func (vpns *VpnS) processWireGuardUpdates(ctx context.Context, rpc pbapi.VpnSClient, vpnsConfig *config.Config) {
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()

	stream, err := rpc.SubscribeWG(subCtx)
	if err != nil {
		vpns.connErrs <- err
		return
	}

	rateLimiter := limiter.NewLimiter(vpnsConfig.TokensPerSecond, vpnsConfig.BucketSize)
	notifications := make(chan *pbapi.WireGuardNotification)
	go func() {
		vpns.log.Debug("Waiting for configuration updates...")
		for {
			notification, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				vpns.log.WithError(err).
					Error("Failed to receive from stream")
				break
			}
			if rateLimiter.Limit() {
				vpns.log.Error("Configuration updates stream rate limit exceeded! Dropping current update..." +
					"(please recheck ratelimiter configuration if this is normal traffic intensity and not DDOS attack)")
				continue
			}
			notifications <- notification
		}
		close(notifications)
	}()

	err = stream.Send(&pbapi.SubscribeWGRequest{Data: &pbapi.SubscribeWGRequest_Subscribe{}})
	if err != nil {
		vpns.connErrs <- err
		return
	}

	for {
		select {
		case ntf, ok := <-notifications:
			if !ok {
				vpns.connErrs <- errors.New("disconnected from update notifications")
				return
			}

			var err error
			if ntf.GetConfigNotification() != nil {
				err = vpns.handleConfigNotification(ntf.GetConfigNotification())
			} else if ntf.GetPeerNotification() != nil {
				err = vpns.handlePeerNotification(ntf.GetPeerNotification())
			} else {
				err = fmt.Errorf("unsupported notification type (%T)", ntf.GetData())
			}
			code := pbapi.RPCErrorCode_OK
			if err != nil {
				code = pbapi.RPCErrorCode_INTERNAL
				vpns.log.Error(err)
			}
			e := stream.Send(updateWGResponse(code, err))
			if e != nil {
				vpns.connErrs <- e
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (vpns *VpnS) handleConfigNotification(wgConfig *pbapi.WireGuard) error {
	vpns.log.Debug("Updating server's wireguard configuration from orchestrator notification...")
	if err := vpns.reconfigureWireGuardVPN(wgConfig); err != nil {
		return fmt.Errorf("could not update server configuration from notification: %w", err)
	}
	return nil
}

func (vpns *VpnS) handlePeerNotification(ntf *pbapi.WireGuardPeerNotification) error {
	vpns.log.Debug("Updating server's wireguard peers from orchestrator notification...")
	switch ntf.Operation {
	case pbapi.WireGuardPeerNotification_UNDEFINED:
		return fmt.Errorf("peer notification type is undefined, ignoring")

	case pbapi.WireGuardPeerNotification_ADD_PEER:
		vpns.log.WithFields(log.Fields{
			"publicKey":  ntf.Peer.PublicKey,
			"allowedIPs": ntf.Peer.AllowedIps,
		}).Info("Adding peer")
		err := vpns.wgc.AddPeers(ntf.Peer)
		if err != nil {
			return fmt.Errorf("could not add peer: %w", err)
		}
		peers := vpns.vpnCfg.GetWg().GetPeers()
		peers = append(peers, ntf.Peer)
		vpns.vpnCfg.GetWg().Peers = peers

	case pbapi.WireGuardPeerNotification_DEL_PEER:
		vpns.log.WithField("publicKey", ntf.Peer.PublicKey).
			Info("Deleting peer")
		// find peer from notification in wg config
		// (notification config has not all peer fields filled(only publickey is filled)! Will use peer from wg config)
		peers := vpns.vpnCfg.GetWg().GetPeers()
		foundIndex := -1
		for i := range peers {
			if peers[i].PublicKey == ntf.Peer.PublicKey {
				foundIndex = i
				break
			}
		}

		// remove peer
		if foundIndex == -1 {
			return errors.New("peer from peer removal notification was not found in wg config")
		}
		err := vpns.wgc.DelPeers(peers[foundIndex])
		if err != nil {
			return fmt.Errorf("could not remove peer: %w", err)
		}

		// update wg configuration
		peers[foundIndex] = peers[len(peers)-1]
		peers = peers[:len(peers)-1]
		vpns.vpnCfg.GetWg().Peers = peers

	default:
		return fmt.Errorf("unsupported peer notification type")
	}
	return nil
}

func (vpns *VpnS) startSendingTelemetry(ctx context.Context, client pbapi.VpnSClient) error {
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return err
	}

	list, err := net.ListenUnix("unix", addr)
	if err != nil {
		return err
	}

	err = list.SetDeadline(time.Now().Add(listenerTimeout))
	if err != nil {
		return err
	}

	vpns.log.Debug("Listening for Telegraf unix socket connection")
	conn, err := list.AcceptUnix()
	if err != nil {
		return err
	}

	stream, err := client.SendTelemetry(ctx)
	if err != nil {
		return err
	}

	go func() {
		logger := vpns.log.WithField("reportCaller", "metrics")
		logger.Info("Starting sending telemetry")
		buf := make([]byte, bufferSize)

		defer func() {
			logger.Info("Closing telemetry connection")
			logger.Debug("Closing gRPC client telemetry stream")
			_, err := stream.CloseAndRecv()
			if err != nil {
				logger.WithError(err).
					Error("Closing gRPC client telemetry stream got error")
			}
			logger.Debug("Closing unix socket connection")
			err = conn.Close()
			if err != nil {
				logger.WithError(err).
					Error("Closing unix socket connection got error")
			}
			err = list.Close()
			if err != nil {
				logger.WithError(err).
					Error("Closing unix listener got error")
			}

			vpns.telemetryStopped <- struct{}{}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, _, err := conn.ReadFromUnix(buf)
				if err != nil {
					logger.WithError(err).
						Error("Failed to read from unix socket")
					time.Sleep(sleepTimeOnError)
				} else {
					if err := stream.Send(&pbapi.WireGuardTelemetry{Data: buf[:n]}); err != nil {
						if err == io.EOF {
							logger.Error("Failed to send telemetry data, the stream is closed")
							time.Sleep(sleepTimeOnError)
							continue
						}
						logger.WithError(err).
							WithField("bytesOfTelemetryData", n).
							Error("Failed to send telemetry data")
						time.Sleep(sleepTimeOnError)
					}
				}
			}
		}
	}()

	return nil
}

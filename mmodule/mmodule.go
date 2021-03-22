package mmodule

import (
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vxcontrol/vxcommon/agent"
	"github.com/vxcontrol/vxcommon/loader"
	"github.com/vxcontrol/vxcommon/utils"
	"github.com/vxcontrol/vxcommon/vxproto"
)

// MainModule is struct which contains full state for agent working
type MainModule struct {
	proto            vxproto.IVXProto
	connectionString string
	agentID          string
	modules          map[string]*loader.ModuleConfig
	loader           loader.ILoader
	socket           vxproto.IModuleSocket
	wgReceiver       sync.WaitGroup
	hasStopped       bool
	mutexResp        *sync.Mutex
}

// OnConnect is function that control hanshake on agent
func (mm *MainModule) OnConnect(socket vxproto.IAgentSocket) (err error) {
	pubInfo := socket.GetPublicInfo()
	logrus.WithFields(logrus.Fields{
		"module": "main",
		"id":     pubInfo.ID,
		"type":   pubInfo.Type.String(),
		"src":    pubInfo.Src,
		"dst":    pubInfo.Dst,
	}).Info("vxagent: connect")
	err = utils.DoHandshakeWithServerOnAgent(socket)
	if err != nil {
		logrus.WithError(err).Error("vxagent: connect error")
	}

	return
}

// DefaultRecvPacket is function that operate packets as default receiver
func (mm *MainModule) DefaultRecvPacket(packet *vxproto.Packet) error {
	logrus.WithFields(logrus.Fields{
		"module": packet.Module,
		"type":   packet.PType.String(),
		"src":    packet.Src,
		"dst":    packet.Dst,
	}).Debug("vxagent: default receiver got new packet")

	return nil
}

// HasAgentIDValid is function that validate AgentID and AngetType in whitelist
func (mm *MainModule) HasAgentIDValid(agentID string, agentType vxproto.AgentType) bool {
	// TODO: here need to implement a check function
	return agentID != ""
}

// HasAgentInfoValid is function that validate Agent Information in Agent list
func (mm *MainModule) HasAgentInfoValid(agentID string, info *agent.Information) bool {
	return true
}

func (mm *MainModule) recvData(src string, data *vxproto.Data) error {
	logrus.WithFields(logrus.Fields{
		"module": "main",
		"type":   "data",
		"src":    src,
		"len":    len(data.Data),
	}).Debug("vxagent: received data")
	err := mm.serveData(src, data)
	logger := logrus.WithFields(logrus.Fields{
		"module": "main",
		"type":   "command",
		"src":    src,
	})
	if err != nil {
		logger.WithError(err).Error("vxagent: failed to exec command")
	} else {
		logger.Debug("vxagent: successful exec server command")
	}

	return nil
}

func (mm *MainModule) recvFile(src string, file *vxproto.File) error {
	logrus.WithFields(logrus.Fields{
		"module": "main",
		"type":   "file",
		"name":   file.Name,
		"path":   file.Path,
		"uniq":   file.Uniq,
		"src":    src,
	}).Debug("vxagent: received file")

	return nil
}

func (mm *MainModule) recvText(src string, text *vxproto.Text) error {
	logrus.WithFields(logrus.Fields{
		"module": "main",
		"type":   "text",
		"name":   text.Name,
		"len":    len(text.Data),
		"src":    src,
	}).Debug("vxagent: received text")

	return nil
}

func (mm *MainModule) recvMsg(src string, msg *vxproto.Msg) error {
	logrus.WithFields(logrus.Fields{
		"module": "main",
		"type":   "msg",
		"msg":    msg.MType.String(),
		"len":    len(msg.Data),
		"src":    src,
	}).Debug("vxagent: received message")

	return nil
}

// New is function which constructed MainModule object
func New(connectionString, agentID string) *MainModule {
	return &MainModule{
		connectionString: connectionString,
		agentID:          agentID,
		modules:          make(map[string]*loader.ModuleConfig),
		loader:           loader.New(),
		mutexResp:        &sync.Mutex{},
	}
}

// Start is function which execute main logic of MainModule
func (mm *MainModule) Start() error {
	mm.hasStopped = false

	mm.proto = vxproto.New(mm)
	if mm.proto == nil {
		return nil
	}

	mm.socket = mm.proto.NewModule("main", mm.agentID)
	if mm.socket == nil {
		return nil
	}

	if !mm.proto.AddModule(mm.socket) {
		err := errors.New("failed register socket for main module")
		logrus.WithError(err).Error("vxagent: add main module failed")
		return err
	}

	// Run main handler of packets
	mm.wgReceiver.Add(1)
	go mm.recvPacket()
	logrus.Debug("vxagent: main module was started")
	defer logrus.Debug("vxagent: main module was stopped")

	config := map[string]string{
		"id":         mm.agentID,
		"token":      "",
		"connection": mm.connectionString,
	}
	for {
		if mm.proto == nil {
			break
		}
		err := mm.proto.Connect(config)
		if mm.hasStopped {
			break
		}
		logrus.WithError(err).Warn("vxagent: try reconnect")
		time.Sleep(time.Second * time.Duration(5))
	}
	return nil
}

// Stop is function which stop main logic of MainModule
func (mm *MainModule) Stop() error {
	mm.hasStopped = true
	logrus.Debug("vxagent: trying to stop main module")
	defer logrus.Info("vxagent: stopping of main module has done")

	if mm.proto == nil {
		return errors.New("VXProto didn't initialized")
	}
	if mm.socket == nil {
		return errors.New("module socket didn't initialized")
	}

	if !mm.proto.DelModule(mm.socket) {
		return errors.New("failed delete module socket")
	}

	if err := mm.loader.StopAll(); err != nil {
		return errors.New("modules didn't stop: " + err.Error())
	}

	for _, id := range mm.loader.List() {
		if !mm.loader.Del(id) {
			return errors.New("failed delete module from loader")
		}
	}

	receiver := mm.socket.GetReceiver()
	if receiver != nil {
		receiver <- &vxproto.Packet{
			PType: vxproto.PTControl,
			Payload: &vxproto.ControlMessage{
				MsgType: vxproto.StopModule,
			},
		}
	}
	mm.wgReceiver.Wait()

	if err := mm.proto.Close(); err != nil {
		return err
	}

	mm.modules = make(map[string]*loader.ModuleConfig)
	mm.proto = nil
	mm.socket = nil

	return nil
}

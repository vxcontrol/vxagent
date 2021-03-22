package mmodule

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
	"github.com/vxcontrol/vxcommon/agent"
	"github.com/vxcontrol/vxcommon/loader"
	"github.com/vxcontrol/vxcommon/utils"
	"github.com/vxcontrol/vxcommon/vxproto"
)

// responseAgent is function which send response to server
func (mm *MainModule) getStatusModules() *agent.ModuleStatusList {
	var modulesList agent.ModuleStatusList
	for _, id := range mm.loader.List() {
		ms := mm.loader.Get(id)
		if ms == nil {
			continue
		}

		mc, ok := mm.modules[id]
		if !ok {
			continue
		}

		var osList []*agent.Config_OS
		for osType, archList := range mc.OS {
			osList = append(osList, &agent.Config_OS{
				Type: utils.GetRef(osType),
				Arch: archList,
			})
		}
		config := &agent.Config{
			Os:         osList,
			AgentId:    utils.GetRef(mc.AgentID),
			Name:       utils.GetRef(mc.Name),
			Version:    utils.GetRef(mc.Version),
			Events:     mc.Events,
			LastUpdate: utils.GetRef(mc.LastUpdate),
		}

		iconfig := &agent.ConfigItem{
			ConfigSchema:       utils.GetRef(mc.GetConfigSchema()),
			DefaultConfig:      utils.GetRef(mc.GetDefaultConfig()),
			CurrentConfig:      utils.GetRef(mc.GetCurrentConfig()),
			EventDataSchema:    utils.GetRef(mc.GetEventDataSchema()),
			EventConfigSchema:  utils.GetRef(mc.GetEventConfigSchema()),
			DefaultEventConfig: utils.GetRef(mc.GetDefaultEventConfig()),
			CurrentEventConfig: utils.GetRef(mc.GetCurrentEventConfig()),
		}

		moduleStatus := &agent.ModuleStatus{
			Name:       utils.GetRef(mc.Name),
			Config:     config,
			ConfigItem: iconfig,
			Status:     ms.GetStatus().Enum(),
		}
		modulesList.List = append(modulesList.List, moduleStatus)
	}

	return &modulesList
}

func (mm *MainModule) getModuleConfig(m *agent.Module) *loader.ModuleConfig {
	mci := &loader.ModuleConfigItem{
		ConfigSchema:       m.GetConfigItem().GetConfigSchema(),
		CurrentConfig:      m.GetConfigItem().GetCurrentConfig(),
		DefaultConfig:      m.GetConfigItem().GetDefaultConfig(),
		EventDataSchema:    m.GetConfigItem().GetEventDataSchema(),
		EventConfigSchema:  m.GetConfigItem().GetEventConfigSchema(),
		DefaultEventConfig: m.GetConfigItem().GetDefaultEventConfig(),
		CurrentEventConfig: m.GetConfigItem().GetCurrentEventConfig(),
	}

	osMap := make(map[string][]string)
	for _, os := range m.GetConfig().GetOs() {
		osMap[os.GetType()] = os.GetArch()
	}
	mc := &loader.ModuleConfig{
		OS:          osMap,
		AgentID:     m.GetConfig().GetAgentId(),
		Name:        m.GetConfig().GetName(),
		Version:     m.GetConfig().GetVersion(),
		Events:      m.GetConfig().GetEvents(),
		LastUpdate:  m.GetConfig().GetLastUpdate(),
		IConfigItem: mci,
	}

	return mc
}

func (mm *MainModule) getModuleItem(m *agent.Module) *loader.ModuleItem {
	mi := loader.NewItem()

	mf := make(map[string][]byte)
	for _, f := range m.GetFiles() {
		mf[f.GetPath()] = f.GetData()
	}
	mi.SetFiles(mf)

	ma := make(map[string][]string)
	for _, a := range m.GetArgs() {
		ma[a.GetKey()] = a.GetValue()
	}
	mi.SetArgs(ma)

	return mi
}

// responseAgent is function which send response to server
func (mm *MainModule) responseAgent(dst string, msgType agent.Message_Type, payload []byte) error {
	mm.mutexResp.Lock()
	defer mm.mutexResp.Unlock()

	if mm.socket == nil {
		return errors.New("module Socket didn't initialize")
	}

	messageData, err := proto.Marshal(&agent.Message{
		Type:    msgType.Enum(),
		Payload: payload,
	})
	if err != nil {
		return errors.New("error marshal request packet: " + err.Error())
	}

	data := &vxproto.Data{
		Data: messageData,
	}
	if err = mm.socket.SendDataTo(dst, data); err != nil {
		return err
	}

	return nil
}

func (mm *MainModule) sendInformation(dst string) error {
	infoMessageData, err := proto.Marshal(utils.GetAgentInformation())
	if err != nil {
		return err
	}

	return mm.responseAgent(dst, agent.Message_INFORMATION_RESULT, infoMessageData)
}

func (mm *MainModule) sendStatusModules(dst string) error {
	statusModulesData, err := proto.Marshal(mm.getStatusModules())
	if err != nil {
		return err
	}

	return mm.responseAgent(dst, agent.Message_STATUS_MODULES_RESULT, statusModulesData)
}

func (mm *MainModule) serveStartModules(dst string, data []byte) (err error) {
	defer func() {
		if errSend := mm.sendStatusModules(dst); errSend != nil {
			if err == nil {
				err = errSend
			} else {
				err = errors.New(err.Error() + " | " + errSend.Error())
			}
		}
	}()

	var moduleList agent.ModuleList
	if err = proto.Unmarshal(data, &moduleList); err != nil {
		err = errors.New("error unmarshal of modules information: " + err.Error())
		return
	}

	for _, m := range moduleList.GetList() {
		var s *loader.ModuleState
		id := m.GetName()
		mc := mm.getModuleConfig(m)
		mi := mm.getModuleItem(m)
		s, err = loader.NewState(mc, mi, mm.proto)
		if err != nil {
			return
		}

		if mm.loader.Get(id) != nil {
			err = errors.New("module " + id + " already exists")
			return
		}

		if !mm.loader.Add(id, s) {
			err = errors.New("failed add module " + id + " to loader")
			return
		}

		if err = mm.loader.Start(id); err != nil {
			return
		}

		mm.modules[id] = mc
	}

	return
}

func (mm *MainModule) serveStopModules(dst string, data []byte) (err error) {
	defer func() {
		if errSend := mm.sendStatusModules(dst); errSend != nil {
			if err == nil {
				err = errSend
			} else {
				err = errors.New(err.Error() + " | " + errSend.Error())
			}
		}
	}()

	var moduleList agent.ModuleList
	if err = proto.Unmarshal(data, &moduleList); err != nil {
		err = errors.New("error unmarshal of modules information: " + err.Error())
		return
	}

	for _, m := range moduleList.GetList() {
		id := m.GetName()
		if mm.loader.Get(id) == nil {
			err = errors.New("module " + id + " not found")
			return
		}

		if err = mm.loader.Stop(id); err != nil {
			return
		}

		if !mm.loader.Del(id) {
			err = errors.New("failed delete module " + id + " from loader")
			return
		}

		delete(mm.modules, id)
	}

	return
}

func (mm *MainModule) serveUpdateModules(dst string, data []byte) (err error) {
	defer func() {
		if errSend := mm.sendStatusModules(dst); errSend != nil {
			if err == nil {
				err = errSend
			} else {
				err = errors.New(err.Error() + " | " + errSend.Error())
			}
		}
	}()

	var moduleList agent.ModuleList
	if err = proto.Unmarshal(data, &moduleList); err != nil {
		err = errors.New("error unmarshal of modules information: " + err.Error())
		return
	}

	for _, m := range moduleList.GetList() {
		id := m.GetName()
		if mm.loader.Get(id) == nil {
			err = errors.New("module " + id + " not found")
			return
		}

		if err = mm.loader.Stop(id); err != nil {
			return
		}

		if !mm.loader.Del(id) {
			err = errors.New("failed delete module " + id + " from loader")
			return
		}

		var s *loader.ModuleState
		mc := mm.getModuleConfig(m)
		mi := mm.getModuleItem(m)
		s, err = loader.NewState(mc, mi, mm.proto)
		if err != nil {
			return
		}

		if !mm.loader.Add(id, s) {
			err = errors.New("failed add module " + id + " to loader")
			return
		}

		if err = mm.loader.Start(id); err != nil {
			return
		}

		mm.modules[id] = mc
	}

	return
}

func (mm *MainModule) serveUpdateConfigModules(dst string, data []byte) (err error) {
	defer func() {
		if errSend := mm.sendStatusModules(dst); errSend != nil {
			if err == nil {
				err = errSend
			} else {
				err = errors.New(err.Error() + " | " + errSend.Error())
			}
		}
	}()

	var moduleList agent.ModuleList
	if err = proto.Unmarshal(data, &moduleList); err != nil {
		err = errors.New("error unmarshal of modules information: " + err.Error())
		return
	}

	for _, m := range moduleList.GetList() {
		id := m.GetName()
		mc := mm.getModuleConfig(m)
		ms := mm.loader.Get(id)
		if ms == nil {
			err = errors.New("module " + id + " not found")
			return
		}

		if romc, ok := mm.modules[id]; ok {
			omc := romc.IConfigItem.(*loader.ModuleConfigItem)
			omc.ConfigSchema = mc.GetConfigSchema()
			omc.DefaultConfig = mc.GetDefaultConfig()
			omc.CurrentConfig = mc.GetCurrentConfig()
			omc.EventDataSchema = mc.GetEventDataSchema()
			omc.EventConfigSchema = mc.GetEventConfigSchema()
			omc.DefaultEventConfig = mc.GetDefaultEventConfig()
			omc.CurrentEventConfig = mc.GetCurrentEventConfig()
		}

		mm.modules[id] = mc
		ms.GetModule().ControlMsg("update_config", mc.GetCurrentConfig())
	}

	return
}

func (mm *MainModule) serveData(src string, data *vxproto.Data) error {
	var message agent.Message
	if err := proto.Unmarshal(data.Data, &message); err != nil {
		return err
	}

	switch message.GetType() {
	case agent.Message_GET_INFORMATION:
		return mm.sendInformation(src)
	case agent.Message_GET_STATUS_MODULES:
		return mm.sendStatusModules(src)
	case agent.Message_START_MODULES:
		return mm.serveStartModules(src, message.Payload)
	case agent.Message_STOP_MODULES:
		return mm.serveStopModules(src, message.Payload)
	case agent.Message_UPDATE_MODULES:
		return mm.serveUpdateModules(src, message.Payload)
	case agent.Message_UPDATE_CONFIG_MODULES:
		return mm.serveUpdateConfigModules(src, message.Payload)
	default:
		return errors.New("received unknown message type")
	}
}

func (mm *MainModule) recvPacket() error {
	defer mm.wgReceiver.Done()

	receiver := mm.socket.GetReceiver()
	if receiver == nil {
		logrus.Error("vxagent: failed to initialize packet receiver")
		return errors.New("failed to initialize packet receiver")
	}
	getAgentEntry := func(agentInfo *vxproto.AgentInfo) *logrus.Entry {
		return logrus.WithFields(logrus.Fields{
			"id":   agentInfo.ID,
			"type": agentInfo.Type.String(),
			"ip":   agentInfo.IP,
			"src":  agentInfo.Src,
			"dst":  agentInfo.Dst,
		})
	}
	for {
		packet := <-receiver
		if packet == nil {
			logrus.Error("vxagent: failed receive packet")
			return errors.New("failed receive packet")
		}
		switch packet.PType {
		case vxproto.PTData:
			mm.recvData(packet.Src, packet.GetData())
		case vxproto.PTFile:
			mm.recvFile(packet.Src, packet.GetFile())
		case vxproto.PTText:
			mm.recvText(packet.Src, packet.GetText())
		case vxproto.PTMsg:
			mm.recvMsg(packet.Src, packet.GetMsg())
		case vxproto.PTControl:
			msg := packet.GetControlMsg()
			switch msg.MsgType {
			case vxproto.AgentConnected:
				getAgentEntry(msg.AgentInfo).Info("vxagent: agent connected")
			case vxproto.AgentDisconnected:
				getAgentEntry(msg.AgentInfo).Info("vxagent: agent disconnected")
			case vxproto.StopModule:
				logrus.Info("vxagent: got signal to stop main module")
				return nil
			}
		default:
			logrus.Error("vxagent: got packet has unexpected packet type")
			return errors.New("unexpected packet type")
		}
	}
}

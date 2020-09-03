// Copyright 2019 The Vearch Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	"github.com/vearch/vearch/util/log"
	"go.etcd.io/etcd/embed"
)

// Model start up model, include all, master, ps, router
type Model int

var single *Config

// Conf return the single instance of config
func Conf() *Config {
	return single
}

var (
	versionOnce  sync.Once
	buildVersion = "0.0"
	buildTime    = "0"
	commitID     = "xxxxx"
)

// SetConfigVersion set the version, time and commit id of build
func SetConfigVersion(bv, bt, ci string) {
	versionOnce.Do(func() {
		buildVersion = bv
		buildTime = bt
		commitID = ci
	})
}

func GetBuildVersion() string {
	return buildVersion
}
func GetBuildTime() string {
	return buildTime
}
func GetCommitID() string {
	return commitID
}

const (
	Master Model = iota
	PS
	Router
)

const (
	LocalSingleAddr = "127.0.0.1"
	LocalCastAddr   = "0.0.0.0"
)

type Config struct {
	Global     *GlobalCfg `toml:"global,omitempty" json:"global"`
	EtcdConfig *EtcdCfg   `toml:"etcd,omitempty" json:"etcd"`
	Masters    Masters    `toml:"masters,omitempty" json:"masters"`
	Router     *RouterCfg `toml:"router,omitempty" json:"router"`
	PS         *PSCfg     `toml:"ps,omitempty" json:"ps"`
}

//get etcd address config
func (this *Config) GetEtcdAddress() []string {
	// depend manageEtcd config
	if this.Global.SelfManageEtcd {
		// provide etcd config ,address and port
		if len(this.EtcdConfig.AddressList) > 0 && this.EtcdConfig.EtcdClientPort > 0 {
			addrs := make([]string, len(this.EtcdConfig.AddressList))
			for i, s := range this.EtcdConfig.AddressList {
				addrs[i] = s + ":" + cast.ToString(this.EtcdConfig.EtcdClientPort)
				log.Info("outside etcd address is %s", addrs[i])
			}
			return addrs
		} else {
			log.Error("the etcd config is error!")
			return nil
		}
	} else {
		// manage etcd by vearch
		ms := this.Masters
		addrs := make([]string, len(ms))
		for i, m := range ms {
			addrs[i] = m.Address + ":" + cast.ToString(ms[i].EtcdClientPort)
			log.Info("vearch etcd address is %s", addrs[i])
		}
		return addrs
	}
}

func (c *Config) GetLogDir() string {
	return c.Global.Log
}

//make sure it not use in loop
func (c *Config) GetLevel() string {
	return c.Global.Level
}

func (c *Config) GetDataDir() string {
	return c.Global.Data[0]
}

func (c *Config) GetDataDirBySlot(model Model, pid uint32) string {
	s := c.Global.Data
	index := int(pid) % len(s)
	return s[index]
}

func (c *Config) GetDatas() []string {
	return c.Global.Data
}

func (c *Config) GetLogFileNum() int {
	return c.Global.LogFileNum
}

func (c *Config) GetLogFileSize() int {
	return c.Global.LogFileSize
}

type Base struct {
	Log         string   `toml:"log,omitempty" json:"log"`
	Level       string   `toml:"level,omitempty" json:"level"`
	LogFileNum  int      `toml:"log_file_num,omitempty" json:"log_file_num"`
	LogFileSize int      `toml:"log_file_size,omitempty" json:"log_file_size"`
	Data        []string `toml:"data,omitempty" json:"data"`
}

type GlobalCfg struct {
	Base
	Name            string `toml:"name,omitempty" json:"name"`
	Signkey         string `toml:"signkey,omitempty" json:"signkey"`
	SkipAuth        bool   `toml:"skip_auth,omitempty" json:"skip_auth"`
	SelfManageEtcd  bool   `toml:"self_manage_etcd,omitempty" json:"self_manage_etcd"`
	AutoRecoverPs   bool   `toml:"auto_recover_ps,omitempty" json:"auto_recover_ps"`
	SupportEtcdAuth bool   `toml:"support_etcd_auth,omitempty" json:"support_etcd_auth"`
}

type EtcdCfg struct {
	AddressList    []string `toml:"address,omitempty" json:"address"`
	EtcdClientPort uint16   `toml:"etcd_client_port,omitempty" json:"etcd_client_port"`
	Username       string   `toml:"user_name,omitempty" json:"user_name"`
	Password       string   `toml:"password,omitempty" json:"password"`
}

type Masters []*MasterCfg

//new client use this function to get client urls
func (ms Masters) ClientAddress() []string {
	addrs := make([]string, len(ms))
	for i, m := range ms {
		addrs[i] = m.Address + ":" + cast.ToString(ms[i].EtcdClientPort)
	}
	return addrs
}

func (ms Masters) Self() *MasterCfg {
	for _, m := range ms {
		if m.Self {
			return m
		}
	}
	return nil

}

type MasterCfg struct {
	Name           string `toml:"name,omitempty" json:"name"`
	Address        string `toml:"address,omitempty" json:"address"`
	ApiPort        uint16 `toml:"api_port,omitempty" json:"api_port"`
	EtcdPort       uint16 `toml:"etcd_port,omitempty" json:"etcd_port"`
	EtcdPeerPort   uint16 `toml:"etcd_peer_port,omitempty" json:"etcd_peer_port"`
	EtcdClientPort uint16 `toml:"etcd_client_port,omitempty" json:"etcd_client_port"`
	Self           bool   `json:"-"`
	SkipAuth       bool   `toml:"skip_auth,omitempty" json:"skip_auth"`
	PprofPort      uint16 `toml:"pprof_port,omitempty" json:"pprof_port"`
	MonitorPort    uint16 `toml:"monitor_port" json:"monitor_port"`
}

func (m *MasterCfg) ApiUrl() string {
	if m.ApiPort == 80 {
		return "http://" + m.Address
	}
	return "http://" + m.Address + ":" + cast.ToString(m.ApiPort)
}

//GetEmbed will get or generate the etcd configuration
func (config *Config) GetEmbed() (*embed.Config, error) {
	masterCfg := config.Masters.Self()

	if masterCfg == nil {
		return nil, fmt.Errorf("not found master config by this machine, please ip , domain , or url config")
	}

	cfg := embed.NewConfig()
	cfg.Name = masterCfg.Name
	cfg.Dir = config.GetDataDir()
	cfg.WalDir = ""
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.EnablePprof = false
	cfg.PreVote = true
	cfg.StrictReconfigCheck = true
	cfg.TickMs = uint(100)
	cfg.ElectionMs = uint(3000)
	cfg.AutoCompactionMode = "periodic"
	cfg.AutoCompactionRetention = "1"
	cfg.MaxRequestBytes = 33554432
	cfg.QuotaBackendBytes = 8589934592
	cfg.InitialClusterToken = config.Global.Signkey

	//set init url
	buf := bytes.Buffer{}
	for _, m := range config.Masters {
		if buf.Len() > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(m.Name)
		buf.WriteString("=http://")
		buf.WriteString(m.Address)
		buf.WriteString(":")
		buf.WriteString(cast.ToString(masterCfg.EtcdPeerPort))
	}
	cfg.InitialCluster = buf.String()

	if urlAddr, err := url.Parse("http://" + masterCfg.Address + ":" + cast.ToString(masterCfg.EtcdPeerPort)); err != nil {
		return nil, err
	} else {
		cfg.LPUrls = []url.URL{*urlAddr}
		cfg.APUrls = []url.URL{*urlAddr}
	}

	if urlAddr, err := url.Parse("http://" + masterCfg.Address + ":" + cast.ToString(masterCfg.EtcdClientPort)); err != nil {
		return nil, err
	} else {
		cfg.ACUrls = []url.URL{*urlAddr}
		cfg.LCUrls = []url.URL{*urlAddr}
	}

	return cfg, nil
}

type RouterCfg struct {
	Port         uint16 `toml:"port,omitempty" json:"port"`
	PprofPort    uint16 `toml:"pprof_port,omitempty" json:"pprof_port"`
	RpcPort      uint16 `toml:"rpc_port,omitempty" json:"rpc_port"`
	MonitorPort  uint16 `toml:"monitor_port" json:"monitor_port"`
	ConnLimit    int    `toml:"conn_limit" json:"conn_limit"`
	CloseTimeout int64  `toml:"close_timeout" json:"close_timeout"`
}
type PSCfg struct {
	RpcPort                uint16 `toml:"rpc_port,omitempty" json:"rpc_port"`
	RaftHeartbeatPort      uint16 `toml:"raft_heartbeat_port,omitempty" json:"raft_heartbeat_port"`
	RaftReplicatePort      uint16 `toml:"raft_replicate_port,omitempty" json:"raft_replicate_port"`
	RaftHeartbeatInterval  int    `toml:"heartbeat_interval" json:"heartbeat-interval"`
	RaftRetainLogs         uint64 `toml:"raft_retain_logs" json:"raft-retain-logs"`
	RaftReplicaConcurrency int    `toml:"raft_replica_concurrency" json:"raft-replica-concurrency"`
	RaftSnapConcurrency    int    `toml:"raft_snap_concurrency" json:"raft-snap-concurrency"`
	RaftTruncateCount      int64  `toml:"raft_truncate_count" json:"raft-snap-concurrency"`
	EngineDWPTNum          uint64 `toml:"engine_dwpt_num" json:"engine-dwpt-num"`
	MaxSize                int64  `toml:"max_size" json:"max_size"`
	PprofPort              uint16 `toml:"pprof_port" json:"pprof_port"`
	Private                bool   `toml:"private" json:"private"`                         //this ps is private if true you must set machine by dbConfig
	FlushTimeInterval      uint32 `toml:"flush_time_interval" json:"flush_time_interval"` // seconds
	FlushCountThreshold    uint32 `toml:"flush_count_threshold" json:"flush_count_threshold"`
}

func InitConfig(path string) {
	single = &Config{}
	LoadConfig(single, path)
}

func LoadConfig(conf *Config, path string) {
	if len(path) == 0 {
		log.Error("configPath file is empty!")
		os.Exit(-1)
	}
	if _, err := toml.DecodeFile(path, conf); err != nil {
		log.Error("decode:[%s] failed, err:[%s]", path, err.Error())
		os.Exit(-1)
	}
}

//CurrentByMasterNameDomainIp find this machine domain.The main purpose of this function is to find the master from from multiple masters and set it‘s Field:self to true.
//The only criterion for judging is: Is the IP address the same with one of the masters?
func (config *Config) CurrentByMasterNameDomainIp(masterName string) error {

	//find local all ip
	addrMap := config.addrMap()

	var found bool

	for _, m := range config.Masters {
		var domainIP *net.IPAddr
		// check if m.Address is a ip
		match, _ := regexp.MatchString(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$`, m.Address)
		if !match {
			// if not match, search DNS IP by domainName
			tempIP, err := net.ResolveIPAddr("ip", m.Address)
			if err != nil {
				log.Errorf("address [%s] is err", m.Address)
				return err
			}
			domainIP = tempIP
			log.Info("master's name:[%s] master's domain:[%s] and local master's ip:[%s]",
				m.Name, m.Address, domainIP)
		}
		if m.Name == masterName {
			m.Self = true
			found = true
		} else if addrMap[m.Address] || (domainIP != nil && addrMap[domainIP.String()]) {
			log.Info("found local master successfully :master's name:[%s] master's ip:[%s] and local master's name:[%s]", m.Name, m.Address, masterName)
			m.Self = true
			found = true
		} else {
			log.Info("find local master failed:master's name:[%s] master's ip:[%s] and local master's name:[%s]", m.Name, m.Address, masterName)
		}
	}

	if !found {
		return errors.New("None of the masters has the same ip address as current local master server's ip")
	}

	return nil
}

func (config *Config) addrMap() map[string]bool {
	addrMap := map[string]bool{LocalSingleAddr: true, LocalCastAddr: true}
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			match, _ := regexp.MatchString(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$`, addr.String())
			if !match {
				continue
			}
			slit := strings.Split(addr.String(), "/")
			addrMap[slit[0]] = true
		}
	}
	return addrMap
}

func (config *Config) Validate(model Model) error {

	switch model {
	case Master:
		masterNum := 0
		for _, m := range config.Masters {
			if m.Self {
				masterNum++
			}
		}

		if masterNum > 1 {
			return fmt.Errorf("in one machine has two masters")
		}
	case PS:
		if config.PS.EngineDWPTNum == 0 {
			config.PS.EngineDWPTNum = 1
		}
		if config.PS.EngineDWPTNum < 0 || config.PS.EngineDWPTNum > 100 {
			return fmt.Errorf("EngineDWPTNum need gt 0 and le 100")
		}
	}

	return config.validatePath()
}

func (config *Config) validatePath() error {
	if err := os.MkdirAll(config.GetLogDir(), os.ModePerm); err != nil {
		return err
	}
	if err := os.MkdirAll(config.GetDataDir(), os.ModePerm); err != nil {
		return err
	}

	return nil
}

package util

import (
	"easyctl/constant"
	"encoding/json"
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type Server struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type RedisServerList struct {
	Servers []Server
}
type NginxServerList struct {
	Servers []Server
}

type DockerServerList struct {
	Servers []Server
}

type ServerList struct {
	RedisServerList  []Server `yaml:"redis"`
	NginxServerList  []Server `yaml:"nginx"`
	DockerServerList []Server `yaml:"docker"`
	YumServerList    []Server `yaml:"yum"`
	PubServerList    []Server `yaml:"pub"`
}

func (server Server) ExecuteOriginCmd(cmd string) (msg string, exitCode int) {
	session, conErr := server.sshConnect()
	fmt.Printf("%s 执行语句：%s\n", PrintCyanMulti([]string{constant.Shell, constant.Remote, server.Host}), cmd)
	if conErr != nil {
		log.Fatal(conErr)
	}

	defer session.Close()

	combo, runErr := session.CombinedOutput(cmd)

	if runErr != nil {
		e, _ := runErr.(*ssh.ExitError)
		exitCode = e.ExitStatus()
	}
	return string(combo), exitCode
}

func (server Server) ExecuteOriginCmdIgnoreRe(cmd string) bool {
	session, conErr := server.sshConnect()
	fmt.Printf("%s 执行语句：%s\n", PrintCyanMulti([]string{server.Host}), cmd)
	if conErr != nil {
		log.Fatal(conErr)
		return false
	}

	defer session.Close()

	_, runErr := session.CombinedOutput(cmd)

	if runErr != nil {
		return false
	}
	return true
}

// 返回结果，是否成功
func (server Server) RemoteExecuteCmdResult(cmd string) bool {
	session, conErr := server.sshConnect()
	fmt.Printf("%s 执行语句：%s\n\n", PrintCyanMulti([]string{server.Host}), cmd)
	if conErr != nil {
		log.Fatal(conErr)
		return false
	}

	defer session.Close()

	_, runErr := session.CombinedOutput(cmd)

	if runErr != nil {
		return false
	}
	return true
}

func (server Server) FileDetection(filetPath string) bool {
	return server.RemoteExecuteCmdResult(fmt.Sprintf("[ -f %s ]", filetPath))
}

func (server Server) RemoteShellParallel(cmd string, wg *sync.WaitGroup) (msg string, exitCode int) {
	defer wg.Done()
	session, conErr := server.sshConnect()
	PrintActionBanner([]string{server.Host},
		fmt.Sprintf("远程执行: %s", cmd))
	if conErr != nil {
		log.Fatal(conErr)
	}

	defer session.Close()

	combo, runErr := session.CombinedOutput(cmd)

	if runErr != nil {
		e, _ := runErr.(*ssh.ExitError)
		exitCode = e.ExitStatus()
		log.Fatal(runErr.Error())
	}

	return string(combo), exitCode
}

func (server Server) RemoteShellPrint(cmd string) {
	session, conErr := server.sshConnect()
	fmt.Printf("%s 执行语句：%s\n", PrintCyanMulti([]string{server.Host}), cmd)
	if conErr != nil {
		log.Fatal(conErr)
	}

	defer session.Close()

	combo, runErr := session.CombinedOutput(cmd)

	if runErr != nil {
		log.Fatal(runErr.Error())
	}
	fmt.Println(string(combo))
}

func (server Server) RemoteShellReturnStd(cmd string) string {
	session, conErr := server.sshConnect()
	fmt.Printf("%s 执行语句：%s\n\n", PrintCyanMulti([]string{server.Host}), cmd)
	if conErr != nil {
		log.Fatal(conErr)
	}

	defer session.Close()

	combo, runErr := session.CombinedOutput(cmd)

	if runErr != nil {
		log.Fatal(runErr.Error())
	}
	return string(combo)
}

func ParseServerList(yamlPath string) ServerList {
	//fmt.Println("解析...")
	var serverList ServerList
	if f, err := os.Open(yamlPath); err != nil {
		//fmt.Println("open yaml...")
		log.Fatal(err)
	} else {
		//fmt.Println("decode...")
		decodeErr := yaml.NewDecoder(f).Decode(&serverList)
		if decodeErr != nil {
			//fmt.Println("decode failed...")
			log.Fatal(decodeErr)
		}
	}
	//fmt.Println("marshal...")
	_, err := json.Marshal(serverList)
	if err != nil {
		//fmt.Println("marshal failed...")
		log.Fatal(err)
	}
	//fmt.Println("print serverlist...")
	return serverList
}

func (server *Server) sshConnect() (*ssh.Session, error) {
	var (
		auth         []ssh.AuthMethod
		addr         string
		clientConfig *ssh.ClientConfig
		client       *ssh.Client
		session      *ssh.Session
		err          error
	)
	// get auth method
	auth = make([]ssh.AuthMethod, 0)
	auth = append(auth, ssh.Password(server.Password))

	hostKeyCallbk := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}

	clientConfig = &ssh.ClientConfig{
		User: server.Username,
		Auth: auth,
		// Timeout:             30 * time.Second,
		HostKeyCallback: hostKeyCallbk,
	}

	// connet to ssh
	addr = fmt.Sprintf("%s:%s", server.Host, server.Port)

	if client, err = ssh.Dial("tcp", addr, clientConfig); err != nil {
		return nil, err
	}

	// create session
	if session, err = client.NewSession(); err != nil {
		return nil, err
	}

	return session, nil
}

func RemotePackageDetection(packageName string, instance Server) bool {
	_, code := instance.ExecuteOriginCmd(fmt.Sprintf("rpm -qa|grep %s", packageName))
	if code == 0 {
		return true
	}

	return false
}

func RemoteInstallPackage(packageName string, instance Server) bool {
	_, code := instance.ExecuteOriginCmd(fmt.Sprintf("yum install -y %s", packageName))
	if code == 0 {
		return true
	}

	return false
}

// 远程写文件
func RemoteWriteFile(filePath string, b []byte, instance Server) {
	// init sftp
	sftp, err := SftpConnect(instance.Username, instance.Password, instance.Host, instance.Port)
	if err != nil {
		fmt.Println(err.Error())
	}
	dstFile, err := sftp.Create(filePath)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer dstFile.Close()
	dstFile.Write(b)
}

func (server *Server) RemoteWriteFile(filePath string, b []byte) {
	PrintActionBanner([]string{server.Host}, fmt.Sprintf("写文件: %s", filePath))
	// init sftp
	sftp, err := SftpConnect(server.Username, server.Password, server.Host, server.Port)
	if err != nil {
		fmt.Println(err.Error())
	}
	dstFile, err := sftp.Create(filePath)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer dstFile.Close()
	dstFile.Write(b)
}

func RemoteWriteFileParallel(filePath string, b []byte, instance Server, wg *sync.WaitGroup) {
	// init sftp
	sftp, err := SftpConnect(instance.Username, instance.Password, instance.Host, instance.Port)
	if err != nil {
		fmt.Println(err.Error())
	}
	dstFile, err := sftp.Create(filePath)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer dstFile.Close()
	defer wg.Done()
	dstFile.Write(b)
}

func ScpHome(banners Banner, localFilePath string, serverList []Server) {
	file, err := os.OpenFile(localFilePath, os.O_RDONLY, 0666)

	if err != nil {
		log.Fatal(err.Error())
	}

	b, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal(err.Error())
	}
	for _, v := range serverList {
		content := fmt.Sprintf("拷贝%s至%s:%s/%s",
			localFilePath, v.Host, HomeDir(v), FormatFileName(file.Name()))
		PrintActionBanner(append(banners.Symbols, v.Host), content)
		RemoteWriteFile(fmt.Sprintf("%s/%s", HomeDir(v), FormatFileName(file.Name())), b, v)
	}
}

func SftpConnect(user, password, host string, port string) (sftpClient *sftp.Client, err error) { //参数: 远程服务器用户名, 密码, ip, 端口
	auth := make([]ssh.AuthMethod, 0)
	auth = append(auth, ssh.Password(password))

	clientConfig := &ssh.ClientConfig{
		User:    user,
		Auth:    auth,
		Timeout: 30 * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}

	addr := host + ":" + port
	sshClient, err := ssh.Dial("tcp", addr, clientConfig) //连接ssh
	if err != nil {
		fmt.Println("连接ssh失败", err)
		return
	}

	if sftpClient, err = sftp.NewClient(sshClient); err != nil { //创建客户端
		fmt.Println("创建客户端失败", err)
		return
	}

	return
}

func HomeDir(server Server) string {
	switch server.Username {
	case "root":
		return "/root"
	default:
		return fmt.Sprintf("/home/%s", server.Username)
	}
}

func (server Server) HomeDir() string {
	switch server.Username {
	case "root":
		return "/root"
	default:
		return fmt.Sprintf("/home/%s", server.Username)
	}
}

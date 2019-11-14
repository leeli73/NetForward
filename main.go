package main

import(
	"os"
	"io"
	"net"
	"log"
	"time"
	"strings"
	"os/exec"
	"net/http"
	"math/rand"
	"io/ioutil"
	"encoding/json"
)

type config struct {
	WebHost string `json:WebHost`
	WebPort string `json:WebPort`
	Password string `json:Password`
}

type link struct {
	name string
	from string
	to string
}

var NowSessionID string
var Config config
var AllLinkRules = make(map[string]link)
var AllLinks = make(map[string]net.Listener)

func Index(w http.ResponseWriter, r *http.Request){
	if r.Method == "GET" {
		if CheckIdentify(r) {
			f,err := os.OpenFile("web/admin.html", os.O_RDONLY,0600)
			if err != nil{
				log.Println("Can't Read File!")
				return
			}
			defer f.Close()
			HTMLByte,err := ioutil.ReadAll(f)
			if err != nil {
				log.Println("Can't Read File Bytes!")
				return
			}
			w.Write(HTMLByte)
		} else {
			f,err := os.OpenFile("web/login.html", os.O_RDONLY,0600)
			if err != nil{
				log.Println("Can't Read File!")
				return
			}
			defer f.Close()
			HTMLByte,err := ioutil.ReadAll(f)
			if err != nil {
				log.Println("Can't Read File Bytes!")
				return
			}
			w.Write(HTMLByte)
		}
	} else if r.Method == "POST" {
		r.ParseForm()
		apiType := r.PostFormValue("type")
		if apiType == "login" {
			password := r.PostFormValue("password")
			if password == "" || password != Config.Password {
				w.Write([]byte("error"))
			} else {
				sessionID := RandString(16)
				NowSessionID = sessionID
				log.Println("Create New Session: " + NowSessionID)
				w.Write([]byte(NowSessionID))
			}
		} else if apiType == "new" {
			name := r.PostFormValue("name")
			from := r.PostFormValue("from")
			to := r.PostFormValue("to")
			go NewLink(name,from,to)
			w.Write([]byte("new"))
		} else if apiType == "delete" {
			name := r.PostFormValue("name")
			DeleteLink(name)
			w.Write([]byte("delete"))
		} else if apiType == "change" {
			name := r.PostFormValue("name")
			from := r.PostFormValue("from")
			to := r.PostFormValue("to")
			go ChangeLink(name,from,to)
			w.Write([]byte("change"))
		} else if apiType == "getLinks" {
			res := "["
			for _,v := range AllLinkRules{
				res = res + `{"name":"` + v.name + `","from":"` + v.from + `","to":"` + v.to + `"},`
			}
			if res != "["{
				res = res[:len(res)-1]
			}
			res = res + "]"
			w.Write([]byte(res))
		} else if apiType == "getArp" {
			cmd := exec.Command("docker inspect -f '{{.Name}} - {{.NetworkSettings.IPAddress }}' $(docker ps -aq)")
			buf,_ := cmd.Output()
			out := string(buf)
			lines := strings.Split(out,"\n")
			res := "["
			for _,v := range lines{
				res = res + v + ","
			}
			if res != "["{
				res = res[:len(res)-1]
			}
			res = res + "]"
			w.Write([]byte(res))
		}
	}
}

func CheckIdentify(r *http.Request) bool {
	session,err := r.Cookie("session")
	if err != nil{
		return false
	}
	if session == nil || session.Value == "" {
		return false
	}
	if session.Value == NowSessionID {
		return true
	}
	return false
}

func RandString(n int) string {
    const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    var src = rand.NewSource(time.Now().UnixNano())
    const (
        letterIdxBits = 6
        letterIdxMask = 1<<letterIdxBits - 1
        letterIdxMax = 63 / letterIdxBits
    )
    b := make([]byte, n)
    for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
        if remain == 0 {
            cache, remain = src.Int63(), letterIdxMax
        }
        if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
            b[i] = letterBytes[idx]
            i--
        }
        cache >>= letterIdxBits
        remain--
    }
    return string(b)
}

func ChangeLink(name string,from string,to string){
	DeleteLink(name)
	var tempData link
	tempData.name = name
	tempData.from = from
	tempData.to = to
	AllLinkRules[name] = tempData
	SaveData()
	NewLink(name,from,to)
}

func NewLink(name string,from string,to string){
	lis, err := net.Listen("tcp", from)
	if err != nil {
		log.Println(err)
		return
	}
	defer lis.Close()
	var tempData link
	tempData.name = name
	tempData.from = from
	tempData.to = to
	AllLinkRules[name] = tempData
	AllLinks[name] = lis
	log.Println("Create Link " + AllLinkRules[name].name + " From:" + AllLinkRules[name].from + " To:" + AllLinkRules[name].to)
	SaveData()
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("Connect with Error:%v\n", err)
			continue
		}
		go handle(conn,name)
	}
}

func DeleteLink(name string){
	AllLinks[name].Close()
	delete(AllLinks,name)
	delete(AllLinkRules,name)
	log.Println("Delete Link " + AllLinkRules[name].name)
	SaveData()
}

func handle(sconn net.Conn,name string) {
	defer sconn.Close()
	dconn, err := net.Dial("tcp", AllLinkRules[name].to)
	if err != nil {
		log.Printf("Connect %v Faild:%v\n", AllLinkRules[name].to, err)
		return
	}
	ExitChan := make(chan bool, 1)
	go func(sconn net.Conn, dconn net.Conn, Exit chan bool) {
		_, err := io.Copy(dconn, sconn)
		log.Printf("Sent to %v Faild:%v\n", AllLinkRules[name].to, err)
		ExitChan <- true
	}(sconn, dconn, ExitChan)
	go func(sconn net.Conn, dconn net.Conn, Exit chan bool) {
		_, err := io.Copy(sconn, dconn)
		log.Printf("Receive form %v Faild:%v\n", AllLinkRules[name].to, err)
		ExitChan <- true
	}(sconn, dconn, ExitChan)
	<-ExitChan
	dconn.Close()
}

func SaveData(){
	f, err := os.OpenFile("conf/data.data", os.O_WRONLY|os.O_TRUNC, 0600)
    defer f.Close()
    if err != nil {
        log.Println(err.Error())
    } else {
		res := ""
		for _,v := range AllLinkRules{
			res = res + v.name + "=" + v.from + "->" + v.to + "\n"
		}
		f.Write([]byte(res))
	}
}

func Init(){
	var Data,err = ioutil.ReadFile("conf/conf.json")
	if err != nil{
		log.Fatal("Read Log File Error！")
	}
	err = json.Unmarshal(Data,&Config)
	if err != nil{
		log.Fatal("Read Log JSON Error！Please Check if Your Charset is UTF-8!")
	}
	linkData,err := ioutil.ReadFile("conf/data.data")
	if err != nil{
		log.Fatal("Read Data File Error！")
	}
	data := string(linkData)
	lines := strings.Split(data,"\n")
	for _,line := range lines {
		if line == ""{
			continue
		}
		temp := strings.Split(line,"=")
		var templink link
		templink.name = temp[0]
		temp2 := strings.Split(temp[1],"->")
		templink.from = temp2[0]
		templink.to = temp2[1]
		AllLinkRules[templink.name] = templink
	}
	for _,v := range AllLinkRules{
		go NewLink(v.name,v.from,v.to)
	}
}

func main(){
	Init()
	http.Handle("/js/", http.FileServer(http.Dir("web")))
	http.Handle("/css/", http.FileServer(http.Dir("web")))
	http.Handle("/fonts/", http.FileServer(http.Dir("web")))
	http.HandleFunc("/",Index)
	log.Println("NetForward Listening Address " + Config.WebHost + ":" + Config.WebPort + "...")
	if err := http.ListenAndServe(Config.WebHost + ":" + Config.WebPort, nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
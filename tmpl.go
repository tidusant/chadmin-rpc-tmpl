package main

import (
	"encoding/json"

	rpch "github.com/tidusant/chadmin-repo/cuahang"

	"github.com/tidusant/c3m-common/c3mcommon"
	"github.com/tidusant/c3m-common/inflect"
	"github.com/tidusant/c3m-common/log"

	"github.com/tidusant/chadmin-repo/models"

	"flag"
	"fmt"
	"net"
	"net/rpc"
	"strconv"
	"strings"
)

type Arith int

func (t *Arith) Run(data string, result *models.RequestResult) error {
	log.Debugf("Call RPCprod args:" + data)
	*result = models.RequestResult{}
	//parse args
	args := strings.Split(data, "|")

	if len(args) < 3 {
		return nil
	}
	var usex models.UserSession
	usex.Session = args[0]
	usex.Action = args[2]
	info := strings.Split(args[1], "[+]")
	usex.UserID = info[0]
	ShopID := info[1]
	usex.Params = ""
	// time.Sleep(time.Duration(time.Second * 10))
	// log.Debugf("userid test %s", usex.UserID)
	// log.Debugf("shoptid test %s", usex.Shop.ID.Hex())
	if len(args) > 3 {
		usex.Params = args[3]
	}
	shop := rpch.GetShopById(usex.UserID, ShopID)
	if shop.ID.Hex() == "" {
		*result = c3mcommon.ReturnJsonMessage("0", "shop not found", "", "")
	}
	usex.Shop = shop

	if usex.Action == "l" {
		*result = LoadAll(usex)

	} else if usex.Action == "a" {
		*result = Active(usex)

	} else if usex.Action == "i" {
		*result = Install(usex)

	} else if usex.Action == "r" {
		rpch.Rebuild(usex)
		*result = c3mcommon.ReturnJsonMessage("1", "", "Success", "")

	}
	return nil
}

func Active(usex models.UserSession) models.RequestResult {
	//request to builserver gettemplate
	request := "activetemplate|" + usex.Session
	resp := c3mcommon.RequestBuildService(request, "POST", usex.Params+","+usex.Shop.Theme)

	if resp.Status != "1" {
		return c3mcommon.ReturnJsonMessage(resp.Status, resp.Error, resp.Message, "")
	}

	var tmpl models.Template
	json.Unmarshal([]byte(resp.Data), &tmpl)
	//templfolder := viper.GetString("config.templatepath") + "/" + tmpl.Code
	if tmpl.Code == "" {
		return c3mcommon.ReturnJsonMessage("0", "Active theme fail", "", "")
	}

	//update shop test dev
	rpch.UpdateTheme(usex.Shop.ID.Hex(), tmpl.Code)
	usex.Shop.Theme = tmpl.Code
	//create build
	rpch.Rebuild(usex)
	return LoadAll(usex)

}
func Install(usex models.UserSession) models.RequestResult {

	//request to builserver gettemplate
	request := "installtemplate|" + usex.Session
	resp := c3mcommon.RequestBuildService(request, "POST", usex.Params+"|"+usex.Shop.Config.DefaultLang)

	if resp.Status != "1" {
		return c3mcommon.ReturnJsonMessage(resp.Status, resp.Error, resp.Message, "")
	}
	var tmpl models.Template
	json.Unmarshal([]byte(resp.Data), &tmpl)

	if tmpl.Code == "" {
		return c3mcommon.ReturnJsonMessage("0", "Install theme fail", "", "")
	}

	//remove old page
	//rpch.RemoveOldTemplatePage(usex.Shop, tmpl)
	pagestr := tmpl.Pages
	var installedPages []string
	var pages map[string]map[string]string
	json.Unmarshal([]byte(pagestr), &pages)
	for pagename, pagecontent := range pages {

		var newpage models.Page
		newpage.Code = pagename
		newpage.TemplateCode = tmpl.Code
		newpage.ShopID = usex.Shop.ID.Hex()

		// //check page exist:
		// cond = bson.M{"shopid": shop.ID.Hex(), "templatecode": template.Code, "code": code}
		// var oldpage models.Page
		// colpage.Find(cond).One(&oldpage)
		// if oldpage.ID.Hex() != "" {
		// 	//skip if exist
		// 	continue
		// }

		for blockname, blockcontent := range pagecontent {
			if blockcontent != "" {
				var block models.PageBlock
				block.Name = blockname
				lines := strings.Split(blockcontent, "\n")
				for _, line := range lines {
					if len(line) == 0 || line[:1] == "#" {
						continue
					}
					srcArr := strings.Split(line, "::")
					if len(srcArr) > 2 {
						var blockitem models.PageBlockItem
						blockitem.Key = srcArr[0]
						blockitem.Type = srcArr[1]
						blockitem.Value = make(map[string]string)
						blockitem.Value[usex.Shop.Config.DefaultLang] = srcArr[2]
						block.Items = append(block.Items, blockitem)
					}

				}
				newpage.Blocks = append(newpage.Blocks, block)
			}

		}

		var pagelang models.PageLang
		pagelang.Title = inflect.ParameterizeJoin(pagename, " ")
		pagelang.Title = inflect.Titleize(pagelang.Title)
		newpage.Langs = make(map[string]models.PageLang)
		newpage.Langs[usex.Shop.Config.DefaultLang] = pagelang
		rpch.InsertPage(newpage)
		installedPages = append(installedPages, newpage.Code)
	}
	rpch.RemoveUnusedTemplatePage(usex.Shop, tmpl, installedPages)

	tmpl.InstalledIDs = append(tmpl.InstalledIDs, usex.Shop.ID.Hex())
	return LoadAll(usex)

}

func LoadAll(usex models.UserSession) models.RequestResult {
	//request to builserver gettemplate
	request := "getalltemplate|" + usex.Session
	resp := c3mcommon.RequestBuildService(request, "POST", "")

	if resp.Status != "1" {
		return c3mcommon.ReturnJsonMessage(resp.Status, resp.Error, resp.Message, "")
	}

	var items []models.Template
	json.Unmarshal([]byte(resp.Data), &items)
	installeds := "["
	templates := "["

	for _, item := range items {

		ok, _ := c3mcommon.InArray(usex.Shop.ID.Hex(), item.InstalledIDs)
		if ok {
			//check active:

			installeds += "{\"Code\":\"" + item.Code + "\",\"Title\":\"" + item.Title + "\",\"Screenshot\":\"" + item.Code + "\",\"Actived\":" + strconv.FormatBool(usex.Shop.Theme == item.Code) + "},"
		} else {
			templates += "{\"Code\":\"" + item.Code + "\",\"Title\":\"" + item.Title + "\",\"Screenshot\":\"" + item.Code + "\",\"Actived\":false},"
		}
	}
	if len(templates) > 1 {
		templates = templates[:len(templates)-1]
	}
	templates += "]"
	if len(installeds) > 1 {
		installeds = installeds[:len(installeds)-1]
	}
	installeds += "]"
	strrt := `{"Templates":` + templates + `,"Installeds":` + installeds + `}`
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}

func main() {
	var port int
	var debug bool
	flag.IntVar(&port, "port", 9882, "help message for flagname")
	flag.BoolVar(&debug, "debug", false, "Indicates if debug messages should be printed in log files")
	flag.Parse()

	logLevel := log.DebugLevel
	if !debug {
		logLevel = log.InfoLevel

	}

	log.SetOutputFile(fmt.Sprintf("Template-"+strconv.Itoa(port)), logLevel)
	defer log.CloseOutputFile()
	log.RedirectStdOut()

	//init db
	arith := new(Arith)
	rpc.Register(arith)
	log.Infof("running with port:" + strconv.Itoa(port))

	tcpAddr, err := net.ResolveTCPAddr("tcp", ":"+strconv.Itoa(port))
	c3mcommon.CheckError("rpc dail:", err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	c3mcommon.CheckError("rpc init listen", err)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go rpc.ServeConn(conn)
	}
}

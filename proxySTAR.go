package main

//Date 03/08
import (
    "fmt"
    "time"
    "strings"
    "strconv"
    "io/ioutil"
    "encoding/json"
    "net/http"
    "os"
    "io"
    "github.com/satori/go.uuid"
    "os/exec"
    "encoding/pem"
)

var completionURL_value = "pending" //message that pops when STAR clientes asks for the uri if it s too early
var cronTaskID = 0 //counter for crontab
var LifeTime = 0

type cdn_post struct {      /*fields in the STAR client CSR*/
        Csr string   `json:"csr"`
        LifeTime int `json:"lifetime"`
        Validity int `json:"validity"`
}

type csr_struct struct {     //fields that need treatment in the received csr string
        subjectName string
}
type successfull_cert struct { //struct returned when polled at getURI: star/registration/cronTaskID
    status string
    lifetime int
    certificate string
}
//Main handler for STAR client requests
func parseJsonPOST(w http.ResponseWriter, r *http.Request) {
        var t cdn_post
        err := json.NewDecoder(r.Body).Decode(&t)
        if err != nil {
                panic(err)
        }
        block, _ := pem.Decode([]byte(t.Csr))
        if block == nil {
                panic("failed to parse PEM block containing the public key")
        }
        if(t.LifeTime > 365 || t.Validity > 200) { //Note that lifetime units are days but validity is in hours
                fmt.Fprintln(w, "Enter parameters are not valid. Maximum lifetime = 365, maximum validity 200.")
        }else {
                LifeTime = t.LifeTime
                cmdS := decodeCsr(t.Csr)
                csr_fields := parseFieldsOfCsr(cmdS)
                /*fmt.Fprintln(w, "Received parameters are valid: LifeTime: ",t.LifeTime," Validity", t.Validity,
                " Domain:", csr_fields.subjectName)
                */
                createTmpFiles(t.Validity)
                go post_completionURL(cronTaskID, LifeTime,completionURL_value)
                w.WriteHeader(http.StatusCreated)
                fmt.Fprintln(w, "Location: https://certProxy/star/registration/" + strconv.Itoa(cronTaskID))
                //time.Sleep(20000 * time.Millisecond)
                //fmt.Printf("%q",csr_fields) //Uncomment this line for some extra-checking

                callCertbot(csr_fields.subjectName) /*Executes certbot for a certain domain*/

                fmt.Printf("Certbot executed successfully")
                storeIssuedCerts(t.Validity)

                //go post_completionURL(cronTaskID, LifeTime,completionURL_value)
                defer rmTmpFiles() //Removes tmp files, comment this function if you want more information

        }

}
/*
Saves info about every issued certificate using STAR.


*/
func storeIssuedCerts (validity int) {
    var certDirName = completionURL_value //All the cert info is kept under a file named as its uri
    var linkFileName = "link" + strconv.Itoa(cronTaskID)
    var csrFileName = "csr" + strconv.Itoa(cronTaskID)
    var validityFileName = "validity" + strconv.Itoa(cronTaskID)
    var uriFileName = "uri" + strconv.Itoa(cronTaskID)
    var certFileName = "certificate.pem"

    //Creates storage directories
    if _, err := os.Stat("/root/starCerts"); os.IsNotExist(err) {
        err = os.Mkdir("/root/starCerts", 0644)
        if err != nil {
            panic(err)
        }
    }
    err := os.Mkdir("/root/starCerts/" + certDirName, 0644)
    if err != nil {
        panic(err)
    }
    if _, err := os.Stat("/root/starCerts/links"); os.IsNotExist(err) {
        err := os.Mkdir("/root/starCerts/links", 0644)
         if err != nil {
                panic(err)
         }
    }
    //links cronTaskID to URI's uuid
    e, err := os.Create("/root/starCerts/links/" + linkFileName)
    if err != nil {

        panic(err)
    }
    defer e.Close()
    e.WriteString(completionURL_value)


    //Saves the csr
    f, err := os.Create("/root/starCerts/" + certDirName + "/" + csrFileName)
    if err != nil {

        panic(err)
    }
    defer f.Close()
    in, err := os.Open("tmpCsr")
    if err != nil {
        panic(err)
    }
    defer in.Close()
    _, err = io.Copy(f, in)

    //Saves the validity
    g, err := os.Create("/root/starCerts/" + certDirName + "/" + validityFileName)
    if err != nil {

        panic(err)
    }
    defer g.Close()
    g.WriteString(strconv.Itoa(validity))

    //Saves the cert
    h, err := os.Create("/root/starCerts/" + certDirName + "/" + certFileName)
    if err != nil {

        panic(err)
    }
    defer h.Close()
    inCert, err := os.Open("ObtainedCERT.pem")
    if err != nil {
        panic(err)
    }
    defer inCert.Close()
    _, err = io.Copy(h, inCert)


    //Saves the uri
    i, err := os.Create("/root/starCerts/" + certDirName + "/" + uriFileName)
    if err != nil {

        panic(err)
    }
    defer i.Close()
    i.WriteString(completionURL_value)


}

/*
Certbot uses STAR protocol if these files exist.
Their contents are : validity, uuid and lifetime.
*/
func createTmpFiles(validity int) {
  completionURL_value = uuid.NewV4().String()
  //Creates a file with cert validity for local certbot to read and deletes the previous one
  _, noFile := os.Stat("STARValidityCertbot")
   if noFile == nil {
          os.Remove("STARValidityCertbot") //Deletes previous file
  }

  toFileErr := ioutil.WriteFile("STARValidityCertbot", []byte(strconv.Itoa(validity)), 0644)
  if toFileErr != nil {
          panic(toFileErr)
  }
  //Creates a file with cert uuid-URI for local certbot to read and deletes the previous one
  _, noFile = os.Stat("STARUuidCertbot")
   if noFile == nil {
          os.Remove("STARUuidCertbot") //Deletes previous file
  }

  toFileErr = ioutil.WriteFile("STARUuidCertbot", []byte(completionURL_value), 0644)
  if toFileErr != nil {
          panic(toFileErr)
  }
  //Creates a file with cert lifetime for local certbot to read and deletes the previous one
  _, noFile = os.Stat("STARLifeTimeCertbot")
   if noFile == nil {
          os.Remove("STARLifeTimeCertbot") //Deletes previous file
  }

  toFileErr = ioutil.WriteFile("STARLifeTimeCertbot", []byte(strconv.Itoa(LifeTime)), 0644)
  if toFileErr != nil {
          panic(toFileErr)
  }
}

/*
Posts the certificates and keeps serving them at :443/uuid


func post_cert (uri string) {
    //fmt.Printf("\nGET the certificate from: %v", completionURL_value)
    http.HandleFunc("/" + completionURL_value, func(w http.ResponseWriter, r *http.Request) {
       http.ServeFile(w, r, "/root/starCerts/" + uri + "/certificate.pem")
        })

}
*/
/*
Invokes post_completionURL when client asks for the certificate
*/
func post_completionURL (id, lifetime int, uri string) {
        http.HandleFunc("/star/registration/" + strconv.Itoa(id), func (w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)

        _, err := os.Stat("./starCerts/" + uri + "/" + "certificate.pem")
        if err == nil {
         a := successfull_cert{status: "valid", lifetime: lifetime,
             certificate: uri}
        lifetimeDuration, err := time.ParseDuration(strconv.Itoa(lifetime) + "h")
        if err != nil {
          panic (err)
        }
         w.Header().Set("Expires", time.Now().Add(lifetimeDuration).String())
         //w.Header().Set("Content-Type", "application/json")
         fmt.Fprintln(w, a)
       }else {
            w.WriteHeader(http.StatusOK)
            fmt.Fprintln(w, "pending")
        }

        })

}

/*
    Creates a copy of the csr in file tmpCsr and returns the csr as plain text
*/
func decodeCsr (csr string)(cmdS string) {
    f, err := os.Create("tmpCsr")
    if err != nil {
        panic(err)
    }

    _, err2 := f.WriteString(csr)
    defer f.Close()
    if err2 != nil {
        panic(err2)
    }
    cmd, err3:= exec.Command("/bin/sh", "getCsrAsText.sh").Output()
    if err3 != nil {
        panic(err3)
    }
    cmdS = (string)(cmd)
    return cmdS
}
/*
    Decodes the csr so that fields can be read.
    For now it is supposed that the required domain is correct.
*/
func parseFieldsOfCsr(cmd string)(csrFields csr_struct) {  /*Returns and array with each important field of the csr in tmpCsr*/
        fmt.Printf("String: %s FIN string", cmd)

        g := strings.SplitN(cmd, "CN=", 2) // Keeps the common name field in g[1]
        f := strings.FieldsFunc(g[1], func(r rune) bool { //f is an array with the fields requested in csr_struct
                switch r {
                        case ' ', '/', '\n' :
                                return true
                        }
                        return false
        })
        csrFields = csr_struct{     /*if csr_struct changes add the rest of the fields here*/
                subjectName: f[0]}
        return csrFields
}

/*
Executes certbot from certbot/certbot/main.py
Executing certbot using one of its auto executables
can destroy the changes done in it that are required
for STAR so execute using this main.py to keep the
changes :)
*/
func callCertbot(domainName string){
        certbotCommand  := []string{"certbotCall.sh", domainName}
        ex,err := exec.Command("/bin/sh",certbotCommand...).Output()
        if err != nil {
                panic(err)
        }
        fmt.Printf("Ejecucion finalizada %s",ex)

}

/*
Executes last.
starCerts isn't supposed to be deleted unless you
restart the proxy because it contains all the live
information.
If you plan to lauch the proxy but you had
obtained a certificate with STAR before, then
use: sudo rm -rf starCerts
*/
func rmTmpFiles () {
    cronTaskID++ //Counter that serves together with the uuid to make each petition unique
    /*err := os.Remove("certId")
    if err != nil {
        panic (err)
    }
        */
    err := os.Remove("ObtainedCERT.pem")
    if err != nil {
        panic (err)
    }
    err = os.Remove("STARValidityCertbot")
    if err != nil {
        panic(err)
    }
    err = os.Remove("STARLifeTimeCertbot")
    if err != nil {
        panic(err)
    }
    err = os.Remove("STARUuidCertbot")
    if err != nil {
        panic(err)
    }
    err = os.Remove("tmpCsr")
    if err != nil {
        panic(err)
    }

}


func main() {
    fmt.Println("Proxy STAR status in middlebox is: ACTIVE")
    http.HandleFunc("/star/registration", parseJsonPOST)
    err := http.ListenAndServeTLS(":443", "server.crt", "server.key", nil)
    if err != nil {
        panic(err)
    }

}


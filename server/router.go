// Copyright (c) 2019 Romano (Viacoin developer)
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/romanornr/AtomicOTCswap/swaputil"

	"github.com/gorilla/mux"
	"github.com/romanornr/AtomicOTCswap/atomic"
	"github.com/romanornr/AtomicOTCswap/bcoins"
	"github.com/romanornr/AtomicOTCswap/insight"
)

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"result"`
	Error   string      `json:"error"`
}

func createRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/", HomeHandler).Methods("GET")
	r.HandleFunc("/swapkeypair", swapKeyPairSiteHandler).Methods("GET")
	r.HandleFunc("/initiate", InitiateSiteHandler).Methods("GET")
	r.HandleFunc("/audit", AuditSiteHandler).Methods("GET")
	r.HandleFunc("/participate", participateSiteHandler).Methods("GET")
	r.HandleFunc("/redeem", RedemptionSiteHandler).Methods("GET")
	r.HandleFunc("/secret", secretSiteHandler).Methods("GET")

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/audit", AuditHandler).Methods("POST")
	api.HandleFunc("/swapkeypair", swapKeyPairHandler).Methods("POST")
	api.HandleFunc("/initiate", InitiateHandler).Methods("POST")
	api.HandleFunc("/participate", ParticipateHandler).Methods("POST")
	api.HandleFunc("/redeem", RedemptionHandler).Methods("POST")
	api.HandleFunc("/secret", SecretHandler).Methods("POST")
	api.HandleFunc("/broadcast", broadcastHandler).Methods("POST")
	http.Handle("/", r)

	r.PathPrefix("/www/").Handler(http.StripPrefix("/www/", http.FileServer(http.Dir("www"))))

	return r
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	err := tpl.ExecuteTemplate(w, "index.gohtml", nil)
	if err != nil {
		fmt.Println("error template")
	}
}

func InitiateSiteHandler(w http.ResponseWriter, r *http.Request) {
	err := tpl.ExecuteTemplate(w, "initiate.gohtml", nil)
	if err != nil {
		fmt.Println("error template")
	}
}

// initiate a contract by parsing the post request
// it parses the coin symbol, counter party address, amount and the wif
func InitiateHandler(w http.ResponseWriter, req *http.Request) {
	amount, err := strconv.ParseFloat(req.FormValue("amount"), 64)
	if err != nil {
		log.Printf("amount should be a float. example: 0.02")
	}

	contract, err := atomic.Initiate(req.FormValue("coin"), req.FormValue("counterPartyAddr"), amount, req.FormValue("wif"))
	respond(w, contract, err)
}

func participateSiteHandler(w http.ResponseWriter, r *http.Request) {
	err := tpl.ExecuteTemplate(w, "participate.gohtml", nil)
	if err != nil {
		fmt.Println("error template")
	}
}

func ParticipateHandler(w http.ResponseWriter, req *http.Request) {
	amount, err := strconv.ParseFloat(req.FormValue("amount"), 64)
	if err != nil {
		respond(w, nil, err)
	}

	fmt.Println(req.Form)

	contract, err := atomic.Participate(req.FormValue("asset"), req.FormValue("counterPartyAddr"), req.FormValue("wif"), amount, req.FormValue("secretHash"))
	respond(w, contract, err)
}

func RedemptionSiteHandler(w http.ResponseWriter, r *http.Request) {
	err := tpl.ExecuteTemplate(w, "redeem.gohtml", nil)
	if err != nil {
		fmt.Println("error template")
	}
}

func RedemptionHandler(w http.ResponseWriter, req *http.Request) {
	redemption, err := atomic.Redeem(req.FormValue("asset"), req.FormValue("contractHex"), req.FormValue("contractTransaction"), req.FormValue("secret"), req.FormValue("wif"))
	respond(w, redemption, err)
}

func secretSiteHandler(w http.ResponseWriter, _ *http.Request) {
	err := tpl.ExecuteTemplate(w, "secret.gohtml", nil)
	if err != nil {
		log.Printf("error template secret")
	}
}

func SecretHandler(w http.ResponseWriter, req *http.Request) {
	secret, err := atomic.ExtractSecret(req.FormValue("redemptionTransaction"), req.FormValue("secretHash"))
	respond(w, secret, err)
}

func AuditSiteHandler(w http.ResponseWriter, req *http.Request) {
	err := tpl.ExecuteTemplate(w, "audit.gohtml", nil)
	if err != nil {
		fmt.Println("error template")
	}
}

// audit a contract by giving the coin symbol, contract hex and contract transaction
// from the contract which needs to be audited
func AuditHandler(w http.ResponseWriter, req *http.Request) {
	contract, err := atomic.AuditContract(req.FormValue("coin"), req.FormValue("contractHex"), req.FormValue("contractTransaction"))
	respond(w, contract, err)
}

func broadcastHandler(w http.ResponseWriter, req *http.Request) {
	asset := req.FormValue("asset")
	rawTransaction := req.FormValue("rawTransaction")
	coin, err := bcoins.SelectCoin(asset)

	tx := bcoins.Transaction{}
	tx.SignedTx = rawTransaction

	_, transaction, err := insight.BroadcastTransaction(coin, tx)
	respond(w, transaction, err)
}

func swapKeyPairSiteHandler(w http.ResponseWriter, _ *http.Request) {
	err := tpl.ExecuteTemplate(w, "swapKeyPair.gohtml", nil)
	if err != nil {
		fmt.Println("error swapKeyPair template")
	}
}

func swapKeyPairHandler(w http.ResponseWriter, req *http.Request) {
	depositAsset, err := bcoins.SelectCoin(req.FormValue("depositAsset"))
	receivingAsset, err := bcoins.SelectCoin(req.FormValue("receivingAsset"))
	if err != nil {
		respond(w, swaputil.SwapKeyPair{}, err)
		return
	}

	swapKeyPair, err := swaputil.GenerateSwapKeyPair(&depositAsset, &receivingAsset)
	respond(w, swapKeyPair, err)
}

func respond(w http.ResponseWriter, data interface{}, err error) {
	response := Response{Data: data, Success: true}
	if err != nil {
		response.Data = nil
		response.Success = false
		response.Error = err.Error()
		///w.WriteHeader(http.StatusBadRequest)
		fmt.Println(err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	return
}

package server

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	bitcoin "github.com/skycoin/skycoin-exchange/src/server/coin_interface/bitcoin"
	"github.com/skycoin/skycoin-exchange/src/server/wallet"
)

type UtxoManager interface {
	Start(closing chan bool)
	GetUtxo(coinType wallet.CoinType) chan bitcoin.Utxo  // get utxo from utxo pool
	PutUtxo(coinType wallet.CoinType, utxo bitcoin.Utxo) // put utxo into utxo pool
	AddWatchAddress(ct wallet.CoinType, addr string)
}

type ExUtxoManager struct {
	WatchAddress map[wallet.CoinType][]string
	UtxosCh      map[wallet.CoinType]chan bitcoin.Utxo
	UtxoStateMap map[wallet.CoinType]map[string]bitcoin.Utxo
}

func NewUtxoManager(wlt wallet.Wallet, utxoPoolsize int) UtxoManager {
	eum := &ExUtxoManager{
		UtxosCh: map[wallet.CoinType]chan bitcoin.Utxo{
			wallet.Bitcoin: make(chan bitcoin.Utxo, utxoPoolsize),
			wallet.Skycoin: make(chan bitcoin.Utxo, utxoPoolsize),
		},
		UtxoStateMap: map[wallet.CoinType]map[string]bitcoin.Utxo{
			wallet.Bitcoin: make(map[string]bitcoin.Utxo),
			wallet.Skycoin: make(map[string]bitcoin.Utxo)},
		WatchAddress: make(map[wallet.CoinType][]string),
	}

	// add watch addresses
	cts := wlt.GetCoinTypes()
	for _, ct := range cts {
		addrs := wlt.GetAddressEntries(ct)
		for _, addr := range addrs {
			eum.AddWatchAddress(ct, addr.Address)
		}
	}
	return eum
}

func (eum ExUtxoManager) Start(closing chan bool) {
	glog.Info("exchange-server start the utxo manager")
	t := time.Tick(CheckTick)
	for {
		select {
		case <-closing:
			return
		case <-t:
			// check new bitcoin utxos.
			newUtxos, err := eum.checkNewBtcUtxo()
			if err != nil {
				glog.Error(err)
				break
			}

			for _, utxo := range newUtxos {
				glog.Info("new bitcoin utxo:", utxo.GetTxid(), " ", utxo.GetVout(), " ", utxo.GetAmount())
				eum.UtxosCh[wallet.Bitcoin] <- utxo
			}

			// TODO: check new skycoin utxos.
		}
	}
}

func (eum *ExUtxoManager) GetUtxo(ct wallet.CoinType) chan bitcoin.Utxo {
	return eum.UtxosCh[ct]
}

func (eum *ExUtxoManager) PutUtxo(ct wallet.CoinType, utxo bitcoin.Utxo) {
	glog.Info(ct.String(), " utxo put back:", utxo.GetAddress(), " ", utxo.GetTxid(), " ", utxo.GetVout())
	eum.UtxosCh[ct] <- utxo
}

func (eum *ExUtxoManager) AddWatchAddress(ct wallet.CoinType, addr string) {
	glog.Info("watch ", ct.String(), " address:", addr)
	eum.WatchAddress[ct] = append(eum.WatchAddress[ct], addr)
}

func (eum *ExUtxoManager) checkNewBtcUtxo() ([]bitcoin.Utxo, error) {
	latestUtxos, err := bitcoin.GetUnspentOutputs(eum.WatchAddress[wallet.Bitcoin])
	if err != nil {
		return []bitcoin.Utxo{}, err
	}

	latestUxMap := make(map[string]bitcoin.Utxo)
	// do diff
	for _, utxo := range latestUtxos {
		id := fmt.Sprintf("%s:%d", utxo.GetTxid(), utxo.GetVout())
		latestUxMap[id] = utxo
	}

	//get new
	newUtxos := []bitcoin.Utxo{}
	for id, utxo := range latestUxMap {
		if _, ok := eum.UtxoStateMap[wallet.Bitcoin][id]; !ok {
			newUtxos = append(newUtxos, utxo)
		}
	}

	eum.UtxoStateMap[wallet.Bitcoin] = latestUxMap
	return newUtxos, nil
}

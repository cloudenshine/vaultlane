package api

import "faka-gateway/internal/store"

// balanceAdapter 适配 store.Store 到 payment.UserStore/OrderStore
type balanceAdapter struct {
	Store *store.Store
}

func (b balanceAdapter) GetUserByID(id int64) (any, error) {
	return b.Store.GetUserByID(id)
}

func (b balanceAdapter) AdjustBalance(userID int64, amount float64, typ, note string, orderID int64) (float64, error) {
	return b.Store.AdjustBalance(userID, amount, typ, note, orderID)
}

func (b balanceAdapter) GetOrderByTradeNo(tradeNo string) (any, error) {
	return b.Store.GetOrderByTradeNo(tradeNo)
}

package downstreamb

import "context"

// ===== 订单 =====

// GetOpenOrderList 查询订单列表
func (c *Client) GetOpenOrderList(ctx context.Context, req *GetOpenOrderListReq) (*GetOpenOrderListResp, error) {
	var resp GetOpenOrderListResp
	if err := c.execute(ctx, "/api/open/order/list", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOpenOrderDetail 查询订单详情
func (c *Client) GetOpenOrderDetail(ctx context.Context, orderNo string) (*GetOpenOrderDetailResp, error) {
	var resp GetOpenOrderDetailResp
	if err := c.execute(ctx, "/api/open/order/detail", &GetOpenOrderDetailReq{OrderNo: orderNo}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOpenOrderKamList 查询订单卡密列表
func (c *Client) GetOpenOrderKamList(ctx context.Context, orderNo string) (*GetOpenOrderKamListResp, error) {
	var resp GetOpenOrderKamListResp
	if err := c.execute(ctx, "/api/open/order/kam/list", &GetOpenOrderKamListReq{OrderNo: orderNo}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateOpenOrderShip 订单物流发货
func (c *Client) UpdateOpenOrderShip(ctx context.Context, req *UpdateOpenOrderShipReq) error {
	return c.execute(ctx, "/api/open/order/ship", req, nil)
}

// ModifyPrice 修改订单价格
func (c *Client) ModifyPrice(ctx context.Context, req *ModifyPriceReq) error {
	return c.execute(ctx, "/api/open/order/modify/price", req, nil)
}

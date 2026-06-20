package downstreamb

import "context"

// ===== 用户/商家 =====

// GetUserAuthorizeList 查询闲鱼店铺授权列表
func (c *Client) GetUserAuthorizeList(ctx context.Context, req *GetUserAuthorizeListReq) (*GetUserAuthorizeListResp, error) {
	var resp GetUserAuthorizeListResp
	if err := c.execute(ctx, "/api/open/user/authorize/list", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateUserAuthorize 创建店铺授权
func (c *Client) CreateUserAuthorize(ctx context.Context, req *CreateUserAuthorizeReq) (*CreateUserAuthorizeResp, error) {
	var resp CreateUserAuthorizeResp
	if err := c.execute(ctx, "/api/open/user/authorize/create", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteUserAuthorize 删除店铺授权
func (c *Client) DeleteUserAuthorize(ctx context.Context, authorizeID int64) error {
	return c.execute(ctx, "/api/open/user/authorize/delete", &DeleteUserAuthorizeReq{AuthorizeID: authorizeID}, nil)
}

// GetUserSellerList 查询商家列表
func (c *Client) GetUserSellerList(ctx context.Context, req *GetUserSellerListReq) (*GetUserSellerListResp, error) {
	var resp GetUserSellerListResp
	if err := c.execute(ctx, "/api/open/user/seller/list", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateUserSeller 创建商家
func (c *Client) CreateUserSeller(ctx context.Context, req *CreateUserSellerReq) (*CreateUserSellerResp, error) {
	var resp CreateUserSellerResp
	if err := c.execute(ctx, "/api/open/user/seller/create", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteUserSeller 删除商家
func (c *Client) DeleteUserSeller(ctx context.Context, sellerID int64) error {
	return c.execute(ctx, "/api/open/user/seller/delete", &DeleteUserSellerReq{SellerID: sellerID}, nil)
}

package downstreamb

import "context"

// ===== 商品 =====

// GetProductCategoryList 查询商品类目
func (c *Client) GetProductCategoryList(ctx context.Context, req *GetProductCategoryListReq) (*GetProductCategoryListResp, error) {
	var resp GetProductCategoryListResp
	if err := c.execute(ctx, "/api/open/product/category/list", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOpenProductList 查询商品列表
func (c *Client) GetOpenProductList(ctx context.Context, req *GetOpenProductListReq) (*GetOpenProductListResp, error) {
	var resp GetOpenProductListResp
	if err := c.execute(ctx, "/api/open/product/list", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOpenProductDetail 查询商品详情
func (c *Client) GetOpenProductDetail(ctx context.Context, productID int64) (*GetOpenProductDetailResp, error) {
	var resp GetOpenProductDetailResp
	if err := c.execute(ctx, "/api/open/product/detail", &GetOpenProductDetailReq{ProductID: productID}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateOpenProduct 创建商品（单个）
func (c *Client) CreateOpenProduct(ctx context.Context, data *OpenProductData) (*CreateOpenProductResp, error) {
	var resp CreateOpenProductResp
	if err := c.execute(ctx, "/api/open/product/create", &CreateOpenProductReq{OpenProductData: *data}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PublishOpenProduct 上架商品（异步）
func (c *Client) PublishOpenProduct(ctx context.Context, req *PublishOpenProductReq) error {
	return c.execute(ctx, "/api/open/product/publish", req, nil)
}

// SyncOpenProductStock 同步库存
func (c *Client) SyncOpenProductStock(ctx context.Context, req *SyncOpenProductStockReq) error {
	return c.execute(ctx, "/api/open/product/edit/stock", req, nil)
}

// PullOffOpenProduct 下架商品
func (c *Client) PullOffOpenProduct(ctx context.Context, req *PullOffOpenProductReq) error {
	return c.execute(ctx, "/api/open/product/downShelf", req, nil)
}

// DeleteOpenProduct 删除商品
func (c *Client) DeleteOpenProduct(ctx context.Context, productID int64) error {
	return c.execute(ctx, "/api/open/product/delete", &DeleteOpenProductReq{ProductID: productID}, nil)
}

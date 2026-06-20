// Package downstreamb types.go
// 精简版：保留核心字段，去掉复杂的 ReportData（验货宝/文玩/美妆等），
// 因为我们主要做"虚拟商品自动发货"，不涉及实物验货。
package downstreamb

// ===== 商品（Product）=====

type GetProductCategoryListReq struct {
	ItemBizType   int32 `json:"item_biz_type" label:"item_biz_type" validate:"oneof=0 2 10 15 16 19 24 26 35"`
	SpBizType     int32 `json:"sp_biz_type,optional" label:"sp_biz_type" validate:"omitempty,oneof=1 2 3 8 9 16 17 18 19 20 21 22 23 24 25 27 28 29 30 31 33 99"`
	FlashSaleType int32 `json:"flash_sale_type,optional" label:"flash_sale_type" validate:"omitempty,oneof=1 2 3 4 5 6 7 8 99"`
}

type GetProductCategoryListResp struct {
	List []CategoryItem `json:"list"`
}

type CategoryItem struct {
	SpBizType      int32  `json:"sp_biz_type"`
	SpBizName      string `json:"sp_biz_name"`
	ChannelCatID   string `json:"channel_cat_id"`
	ChannelCatName string `json:"channel_cat_name"`
}

type GetOpenProductListReq struct {
	OnlineTime    []int64 `json:"online_time,optional" label:"online_time" validate:"omitempty,len=2"`
	OfflineTime   []int64 `json:"offline_time,optional" label:"offline_time" validate:"omitempty,len=2"`
	SoldTime      []int64 `json:"sold_time,optional" label:"sold_time" validate:"omitempty,len=2"`
	UpdateTime    []int64 `json:"update_time,optional" label:"update_time" validate:"omitempty,len=2"`
	CreateTime    []int64 `json:"create_time,optional" label:"create_time" validate:"omitempty,len=2"`
	ProductStatus int32   `json:"product_status,optional" label:"product_status" validate:"omitempty,oneof=-1 10 21 22 23 31 33 36"`
	SaleStatus    int32   `json:"sale_status,optional" label:"sale_status" validate:"omitempty,oneof=1 2 3"`
	PageNo        int32   `json:"page_no,default=1,optional" label:"page_no" validate:"omitempty,min=1,max=100"`
	PageSize      int32   `json:"page_size,default=50,optional" label:"page_size" validate:"omitempty,min=1,max=100"`
}

type GetOpenProductListResp struct {
	List     []ProductListItem `json:"list"`
	Count    int64             `json:"count"`
	PageNo   int32             `json:"page_no"`
	PageSize int32             `json:"page_size"`
}

type ProductListItem struct {
	ProductID     int64  `json:"product_id"`     // 管家商品 ID
	ProductStatus int32  `json:"product_status"` // 管家商品状态
	ItemBizType   int32  `json:"item_biz_type"`
	SpBizType     int32  `json:"sp_biz_type"`
	ChannelCatID  string `json:"channel_cat_id"`
	OriginalPrice int64  `json:"original_price"` // 分
	Price         int64  `json:"price"`          // 分
	Stock         int32  `json:"stock"`
	Sold          int32  `json:"sold"`
	Title         string `json:"title"`
	DistrictID    int32  `json:"district_id"`
	OuterID       string `json:"outer_id"`
	ExpressFee    int64  `json:"express_fee"`
	SpecType      int32  `json:"spec_type"`
	Source        int32  `json:"source"`
	OnlineTime    int64  `json:"online_time"`
	OfflineTime   int64  `json:"offline_time"`
	SoldTime      int64  `json:"sold_time"`
	UpdateTime    int64  `json:"update_time"`
	CreateTime    int64  `json:"create_time"`
}

type GetOpenProductDetailReq struct {
	ProductID int64 `json:"product_id" validate:"required"`
}

type GetOpenProductDetailResp struct {
	ProductID     int64          `json:"product_id"`
	ProductStatus int32          `json:"product_status"`
	PublishStatus int32          `json:"publish_status"`
	ItemBizType   *int32         `json:"item_biz_type,omitempty"`
	SpBizType     *int32         `json:"sp_biz_type,omitempty"`
	ChannelCatID  *string        `json:"channel_cat_id,omitempty"`
	Title         *string        `json:"title,omitempty"`
	Price         *int64         `json:"price,omitempty"`
	OriginalPrice *int64         `json:"original_price,omitempty"`
	Stock         *int32         `json:"stock,omitempty"`
	Sold          *int32         `json:"sold,omitempty"`
	OuterID       *string        `json:"outer_id,omitempty"`
	ExpressFee    *int64         `json:"express_fee,omitempty"`
	StuffStatus   *int32         `json:"stuff_status,omitempty"`
	SpecType      *int32         `json:"spec_type,omitempty"`
	OnlineTime    *int64         `json:"online_time,omitempty"`
	OfflineTime   *int64         `json:"offline_time,omitempty"`
	SoldTime      *int64         `json:"sold_time,omitempty"`
	UpdateTime    *int64         `json:"update_time,omitempty"`
	CreateTime    *int64         `json:"create_time,omitempty"`
	PublishShop   []*PublishShop `json:"publish_shop,omitempty"`
	UserName      []*string      `json:"user_name,omitempty"`
	SkuItems      []*SkuItem     `json:"sku_items,omitempty"`
	DetailImages  []Image        `json:"detail_images,omitempty"`
	SkuImages     []Image        `json:"sku_images,omitempty"`
	IsTaxIncluded *bool          `json:"is_tax_included,omitempty"`
}

type Image struct {
	Height  int32  `json:"height"`
	Width   int32  `json:"width"`
	Src     string `json:"src"`
	SkuText string `json:"sku_text,omitempty"`
}

type PublishShop struct {
	UserName       string   `json:"user_name" label:"publish_shop.user_name" validate:"required"`
	ItemID         int64    `json:"item_id,optional" label:"publish_shop.item_id" validate:"omitempty"`
	Province       int32    `json:"province,optional" label:"publish_shop.province" validate:"omitempty"`
	City           int32    `json:"city,optional" label:"publish_shop.city" validate:"omitempty"`
	District       int32    `json:"district,optional" label:"publish_shop.district" validate:"omitempty"`
	Title          string   `json:"title" label:"publish_shop.title" validate:"required,min=1,max=60"`
	Content        string   `json:"content" label:"publish_shop.content" validate:"required,min=5,max=5000"`
	Images         []string `json:"images" label:"publish_shop.images" validate:"required,max=30"`
	Status         int32    `json:"status,optional" label:"publish_shop.status" validate:"omitempty"`
	WhiteImages    string   `json:"white_images,optional" label:"publish_shop.white_images" validate:"omitempty"`
	ServiceSupport string   `json:"service_support,optional" label:"publish_shop.service_support" validate:"omitempty"`
}

type SkuItem struct {
	SkuID   int64  `json:"sku_id,optional" label:"sku_items.sku_id" validate:"omitempty"`
	XySkuID int64  `json:"xy_sku_id,optional" label:"sku_items.xy_sku_id" validate:"omitempty"`
	Price   int64  `json:"price,optional" label:"sku_items.price" validate:"omitempty,min=1"`
	Stock   int32  `json:"stock,optional" label:"sku_items.stock" validate:"omitempty,min=0"`
	SkuText string `json:"sku_text,optional" label:"sku_items.sku_text" validate:"omitempty"`
	OuterID string `json:"outer_id,optional" label:"sku_items.outer_id" validate:"omitempty"`
}

type CreateOpenProductReq struct {
	OpenProductData
}

type OpenProductData struct {
	ItemKey       string        `json:"item_key,optional" label:"item_key" validate:"omitempty"`
	ItemBizType   int32         `json:"item_biz_type" label:"item_biz_type" validate:"oneof=0 2 10 15 16 19 24 26 35"`
	SpBizType     int32         `json:"sp_biz_type" label:"sp_biz_type" validate:"oneof=1 2 3 8 9 16 17 18 19 20 21 22 23 24 25 27 28 29 30 31 33 99"`
	ChannelCatID  string        `json:"channel_cat_id" label:"channel_cat_id" validate:"required"`
	Title         string        `json:"title,optional" label:"title" validate:"omitempty"`
	Price         int64         `json:"price" label:"price" validate:"required,min=1"`
	OriginalPrice int64         `json:"original_price,optional" label:"original_price" validate:"omitempty"`
	Stock         int32         `json:"stock" label:"stock" validate:"required,min=1"`
	ExpressFee    int64         `json:"express_fee,optional" label:"express_fee" validate:"omitempty"`
	OuterID       string        `json:"outer_id,optional" label:"outer_id" validate:"omitempty,max=64"`
	PublishShop   []PublishShop `json:"publish_shop" label:"publish_shop" validate:"required,dive"`
	SkuItems      []*SkuItem    `json:"sku_items,optional,omitempty" label:"sku_items" validate:"omitempty,dive"`
	DetailImages  []Image       `json:"detail_images,optional,omitempty" label:"detail_images" validate:"omitempty"`
}

type CreateOpenProductResp struct {
	ProductID     int64 `json:"product_id"`
	ProductStatus int32 `json:"product_status"`
}

type PublishOpenProductReq struct {
	ProductID          int64    `json:"product_id" label:"product_id" validate:"required"`
	UserName           []string `json:"user_name" label:"user_name" validate:"required"`
	SpecifyPublishTime string   `json:"specify_publish_time,optional" label:"specify_publish_time" validate:"omitempty"`
	NotifyUrl          string   `json:"notify_url,optional" label:"notify_url" validate:"omitempty"`
}

type PublishOpenProductResp struct{}

type SyncOpenProductStockReq struct {
	ProductID int64      `json:"product_id" label:"product_id" validate:"required"`
	Price     *int64     `json:"price,optional" label:"price" validate:"omitempty"`
	Stock     *int32     `json:"stock,optional" label:"stock" validate:"omitempty"`
	UserName  []string   `json:"user_name,optional" label:"user_name" validate:"omitempty"`
	SkuItems  []*SkuItem `json:"sku_items,optional" label:"sku_items" validate:"omitempty"`
}

type SyncOpenProductStockResp struct{}

type PullOffOpenProductReq struct {
	ProductID int64    `json:"product_id" label:"product_id" validate:"required"`
	UserName  []string `json:"user_name,optional" label:"user_name" validate:"omitempty"`
}

type PullOffOpenProductResp struct{}

type DeleteOpenProductReq struct {
	ProductID int64 `json:"product_id" label:"product_id" validate:"required"`
}

type DeleteOpenProductResp struct{}

// ===== 订单（Order）=====

type GetOpenOrderListReq struct {
	AuthorizeID  int64   `json:"authorize_id,optional"`
	ConfirmTime  []int64 `json:"confirm_time,optional" validate:"omitempty,len=2"`
	ConsignTime  []int64 `json:"consign_time,optional" validate:"omitempty,len=2"`
	OrderTime    []int64 `json:"order_time,optional" validate:"omitempty,len=2"`
	PayTime      []int64 `json:"pay_time,optional" validate:"omitempty,len=2"`
	RefundTime   []int64 `json:"refund_time,optional" validate:"omitempty,len=2"`
	UpdateTime   []int64 `json:"update_time,optional" validate:"omitempty,len=2"`
	OrderStatus  int32   `json:"order_status,optional" validate:"omitempty,oneof=11 12 21 22 23 24"`
	RefundStatus int32   `json:"refund_status,optional" validate:"omitempty,oneof=0 1 2 3 4 5 6 8"`
	PageNo       int32   `json:"page_no,optional,default=1" validate:"omitempty,min=1,max=100"`
	PageSize     int32   `json:"page_size,optional,default=50" validate:"omitempty,min=1,max=100"`
}

type GetOpenOrderListResp struct {
	List     []GetOpenOrderDetailResp `json:"list"`
	Count    int64                    `json:"count"`
	PageNo   int32                    `json:"page_no"`
	PageSize int32                    `json:"page_size"`
}

type GetOpenOrderDetailReq struct {
	OrderNo string `json:"order_no" label:"order_no" validate:"required,len=19"`
}

type GetOpenOrderDetailResp struct {
	OrderNo         string `json:"order_no"`
	OrderStatus     int32  `json:"order_status"`
	OrderType       int32  `json:"order_type"`
	OrderTime       int64  `json:"order_time"`
	TotalAmount     int64  `json:"total_amount"`
	PayAmount       int64  `json:"pay_amount"`
	PayNo           string `json:"pay_no"`
	PayTime         int64  `json:"pay_time"`
	RefundStatus    int32  `json:"refund_status"`
	RefundAmount    int64  `json:"refund_amount"`
	RefundTime      int64  `json:"refund_time"`
	ReceiverMobile  string `json:"receiver_mobile,omitempty"`
	ReceiverName    string `json:"receiver_name,omitempty"`
	WaybillNo       string `json:"waybill_no"`
	ExpressCode     string `json:"express_code"`
	ExpressName     string `json:"express_name"`
	ConsignType     int32  `json:"consign_type"` // 1 物流 2 虚拟
	ConsignTime     int64  `json:"consign_time"`
	ConfirmTime     int64  `json:"confirm_time"`
	CancelReason    string `json:"cancel_reason"`
	CreateTime      int64  `json:"create_time"`
	UpdateTime      int64  `json:"update_time"`
	BuyerEid        string `json:"buyer_eid"`
	BuyerNick       string `json:"buyer_nick"`
	SellerEid       string `json:"seller_eid"`
	SellerName      string `json:"seller_name"`
	Goods           Goods  `json:"goods"`
	XybSellerAmount int64  `json:"xyb_seller_amount"`
}

type Goods struct {
	Quantity   int32    `json:"quantity"`
	Price      int64    `json:"price"` // 分
	ProductID  int64    `json:"product_id"`
	ItemID     int64    `json:"item_id"`
	OuterID    string   `json:"outer_id"`
	SkuID      int64    `json:"sku_id"`
	SkuOuterID string   `json:"sku_outer_id"`
	SkuText    string   `json:"sku_text"`
	Title      string   `json:"title"`
	Images     []string `json:"images"`
}

type GetOpenOrderKamListReq struct {
	OrderNo string `json:"order_no" validate:"required,len=19"`
}

type GetOpenOrderKamListResp struct {
	List []KamItem `json:"list"`
}

type KamItem struct {
	CardNo   string `json:"card_no"`
	CardPwd  string `json:"card_pwd"`
	Cost     int32  `json:"cost"`
	SoldType int32  `json:"sold_type"`
}

type UpdateOpenOrderShipReq struct {
	OrderNo        string `json:"order_no" label:"order_no" validate:"required,len=19"`
	WaybillNo      string `json:"waybill_no" label:"waybill_no" validate:"required,max=50"`
	ExpressCode    string `json:"express_code" label:"express_code" validate:"required"`
	ExpressName    string `json:"express_name" label:"express_name" validate:"required"`
	ShipAddress    string `json:"ship_address,optional" validate:"omitempty"`
	ShipAreaName   string `json:"ship_area_name,optional" validate:"omitempty"`
	ShipCityName   string `json:"ship_city_name,optional" validate:"omitempty"`
	ShipDistrictID int32  `json:"ship_district_id,optional" validate:"omitempty"`
	ShipMobile     string `json:"ship_mobile,optional" validate:"omitempty,phone"`
	ShipName       string `json:"ship_name,optional" validate:"omitempty"`
	ShipProvName   string `json:"ship_prov_name,optional" validate:"omitempty"`
}

type UpdateOpenOrderShipResp struct{}

type ModifyPriceReq struct {
	OrderNo    string `json:"order_no" label:"order_no" validate:"required,len=19"`
	OrderPrice int64  `json:"order_price" validate:"required,gte=1"`
	ExpressFee int64  `json:"express_fee" validate:"required,gte=0"`
}

type ModifyPriceResp struct{}

// ===== 用户/商家 =====

type GetUserAuthorizeListReq struct {
	AuthorizeID int64 `json:"authorize_id,optional"`
}

type GetUserAuthorizeListResp struct {
	List []AuthorizeItem `json:"list"`
}

type AuthorizeItem struct {
	AuthorizeID      int64  `json:"authorize_id"`
	AuthorizeExpires int64  `json:"authorize_expires"`
	SellerID         int64  `json:"seller_id"`
	UserID           int64  `json:"user_id"`
	UserIdentity     string `json:"user_identity"`
	UserName         string `json:"user_name"`
	UserNick         string `json:"user_nick"`
	ShopName         string `json:"shop_name"`
	IsPro            bool   `json:"is_pro"`
	IsDepositEnough  bool   `json:"is_deposit_enough"`
	ServiceSupport   string `json:"service_support"`
	IsValid          bool   `json:"is_valid"`
	IsTrial          bool   `json:"is_trial"`
	ValidEndTime     int64  `json:"valid_end_time"`
	ValidStartTime   int64  `json:"valid_start_time"`
	ItemBizTypes     string `json:"item_biz_types"`
}

type CreateUserAuthorizeReq struct {
	AuthorizeCode string `json:"authorize_code" validate:"required"`
	ShopName      string `json:"shop_name"`
}

type CreateUserAuthorizeResp struct {
	SellerID         int64  `json:"seller_id"`
	AuthorizeID      int64  `json:"authorize_id"`
	AuthorizeExpires int64  `json:"authorize_expires"`
	UserIdentity     string `json:"user_identity"`
	UserName         string `json:"user_name"`
	UserNick         string `json:"user_nick"`
	ShopName         string `json:"shop_name"`
	ServiceSupport   string `json:"service_support"`
	IsDepositEnough  bool   `json:"is_deposit_enough"`
	IsPro            bool   `json:"is_pro"`
	ItemBizTypes     string `json:"item_biz_types"`
}

type DeleteUserAuthorizeReq struct {
	AuthorizeID int64 `json:"authorize_id" validate:"required"`
}

type DeleteUserAuthorizeResp struct {
	Success bool `json:"success"`
}

type DeleteUserSellerReq struct {
	SellerID int64 `json:"seller_id" validate:"required"`
}

type DeleteUserSellerResp struct {
	Success bool `json:"success"`
}

type CreateUserSellerReq struct {
	SellerName   string `json:"seller_name"`
	SellerMobile string `json:"seller_mobile" validate:"required,phone"`
}

type CreateUserSellerResp struct {
	Seller
}

type Seller struct {
	SellerID     int64  `json:"seller_id"`
	SellerName   string `json:"seller_name"`
	SellerMobile string `json:"seller_mobile"`
	CreateTime   int64  `json:"create_time"`
}

type GetUserSellerListReq struct {
	SellerID     int64   `json:"seller_id,optional"`
	SellerMobile string  `json:"seller_mobile,optional"`
	CreateTime   []int64 `json:"create_time,optional" validate:"omitempty,len=2"`
	PageNo       int32   `json:"page_no,default=1"`
	PageSize     int32   `json:"page_size,default=25"`
}

type GetUserSellerListResp struct {
	List     []Seller `json:"list"`
	Count    int32    `json:"count"`
	PageNo   int32    `json:"page_no"`
	PageSize int32    `json:"page_size"`
}

// ===== 钱包 =====

type WalletAccountReq struct{}

type WalletAccountResp struct {
	AccountBalance int64 `json:"account_balance"`
	UpdateTime     int64 `json:"update_time"`
}

// ===== 回调（推送给我们）=====

// VirtualOrderNotifyReq 货源虚拟订单状态/卡密推送
type VirtualOrderNotifyReq struct {
	SellerID    int64                          `json:"seller_id,optional"`
	AppID       int64                          `json:"app_id,optional"`
	BizID       int64                          `json:"biz_id,optional"`
	OrderType   int32                          `json:"order_type"`
	OrderNo     string                         `json:"order_no"`
	OutOrderNo  string                         `json:"out_order_no"`
	OrderStatus int32                          `json:"order_status"`
	EndTime     int64                          `json:"end_time"`
	CardItems   []VirtualOrderNotifyCardItem   `json:"card_items,optional"`
	TicketItems []VirtualOrderNotifyTicketItem `json:"ticket_items,optional"`
	Remark      string                         `json:"remark,optional"`
}

type VirtualOrderNotifyCardItem struct {
	CardNo  string `json:"card_no"`
	CardPwd string `json:"card_pwd"`
}

type VirtualOrderNotifyTicketItem struct {
	CodeNo         string `json:"code_no"`
	CodePwd        string `json:"code_pwd"`
	CodeName       string `json:"code_name"`
	CodeType       int32  `json:"code_type"`
	ValidStartTime int64  `json:"valid_start_time"`
	ValidEndTime   int64  `json:"valid_end_time"`
	Status         int32  `json:"status"`
}

type VirtualNotifyResp struct {
	Code int32  `json:"code"`
	Msg  string `json:"msg"`
}

// FuluOrderNotifyReq 福禄订单通知（参考）
type FuluOrderNotifyReq struct {
	XyOrderNo            string `json:"customer_order_no,optional"`
	ChargeFinishTime     string `json:"charge_finish_time,optional"`
	OutOrderNo           string `json:"order_id,optional"`
	OrderStatus          string `json:"order_status,optional"`
	RechargeDescription  string `json:"recharge_description,optional"`
	ProductID            string `json:"product_id,optional"`
	Price                string `json:"price,optional"`
	BuyNum               string `json:"buy_num,optional"`
	OperatorSerialNumber string `json:"operator_serial_number,optional"`
	Sign                 string `json:"sign"`
}

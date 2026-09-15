package delivery

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"faka-gateway/internal/config"
	"faka-gateway/internal/store"
)

// EmailSender 真实 SMTP 邮件投递器
type EmailSender struct {
	cfg config.SMTPConfig
}

// NewEmailSender 构造邮件投递器
func NewEmailSender(cfg config.SMTPConfig) *EmailSender {
	return &EmailSender{cfg: cfg}
}

// SendMail 发送标准纯文本/HTML 邮件
func (s *EmailSender) SendMail(to, subject, body string) error {
	if s.cfg.Host == "" || s.cfg.From == "" {
		return nil // 未配置 SMTP 静默忽略
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	from := s.cfg.From
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	header := make(map[string]string)
	header["From"] = from
	header["To"] = to
	header["Subject"] = "=?UTF-8?B?" + subject + "?="
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=UTF-8"
	header["Date"] = time.Now().Format(time.RFC1123Z)

	msg := ""
	for k, v := range header {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + body

	// SSL/TLS (通常是 465 端口)
	if s.cfg.Port == 465 || s.cfg.UseTLS {
		tlsConfig := &tls.Config{
			ServerName:         s.cfg.Host,
			InsecureSkipVerify: false,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls dial smtp error: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			return fmt.Errorf("smtp new client error: %w", err)
		}
		defer client.Close()

		if s.cfg.Username != "" && s.cfg.Password != "" {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth error: %w", err)
			}
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(msg))
		if err != nil {
			return err
		}
		return w.Close()
	}

	// 587 或 25 普通 SMTP / STARTTLS
	_ = net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// SendDeliveryEmail 发送订单卡密给用户
func (s *EmailSender) SendDeliveryEmail(order *store.Order) error {
	if order.Contact == "" || !strings.Contains(order.Contact, "@") {
		return nil
	}
	subject := fmt.Sprintf("【发货通知】您购买的「%s」已自动发货", order.CommodityName)
	body := fmt.Sprintf(`
<div style="font-family: sans-serif; max-width: 600px; margin: 0 auto; border: 1px solid #eaeaea; border-radius: 8px; padding: 24px;">
  <h2 style="color: #1F3A5F; margin-top: 0;">感谢您的购买！</h2>
  <p>您的订单 <strong>%s</strong> 已成功履约发货。</p>
  <div style="background: #F8FAFC; border-left: 4px solid #1F3A5F; padding: 16px; margin: 20px 0; border-radius: 4px;">
    <h3 style="margin-top: 0; color: #334155;">商品名称：%s (共 %d 件)</h3>
    <p style="margin-bottom: 8px; font-weight: bold; color: #475569;">发货卡密内容：</p>
    <pre style="background: #FFFFFF; border: 1px solid #E2E8F0; padding: 12px; border-radius: 4px; font-size: 14px; color: #B23A48; white-space: pre-wrap; word-break: break-all;">%s</pre>
  </div>
  <p style="font-size: 12px; color: #94A3B8;">订单金额：¥%.2f | 下单时间：%s</p>
  <hr style="border: none; border-top: 1px solid #E2E8F0; margin: 24px 0;" />
  <p style="font-size: 12px; color: #64748B;">请妥善保管您的卡密，如有疑问请及时联系客服。</p>
</div>`, order.TradeNo, order.CommodityName, order.Num, order.Contents, order.Amount, order.CreatedAt.Format("2006-01-02 15:04:05"))

	return s.SendMail(order.Contact, subject, body)
}

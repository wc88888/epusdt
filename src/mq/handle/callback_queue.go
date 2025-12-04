package handle

import (
	"context"
	"errors"

	"github.com/assimon/luuu/config"
	"github.com/assimon/luuu/model/data"
	"github.com/assimon/luuu/model/mdb"
	"github.com/assimon/luuu/model/response"
	"github.com/assimon/luuu/util/http_client"
	"github.com/assimon/luuu/util/json"
	"github.com/assimon/luuu/util/log"
	"github.com/assimon/luuu/util/sign"
	"github.com/hibiken/asynq"
)

const QueueOrderCallback = "order:callback"

func NewOrderCallbackQueue(order *mdb.Orders) (*asynq.Task, error) {
	payload, err := json.Cjson.Marshal(order)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(QueueOrderCallback, payload), nil
}

func OrderCallbackHandle(ctx context.Context, t *asynq.Task) error {
	var order mdb.Orders
	err := json.Cjson.Unmarshal(t.Payload(), &order)
	if err != nil {
		return err
	}
	defer func() {
		if err := recover(); err != nil {
			log.Sugar.Error(err)
		}
	}()
	defer func() {
		data.SaveCallBackOrdersResp(&order)
	}()
	client := http_client.GetHttpClient()
	orderResp := response.OrderNotifyResponse{
		TradeId:            order.TradeId,
		OrderId:            order.OrderId,
		Amount:             order.Amount,
		ActualAmount:       order.ActualAmount,
		Token:              order.Token,
		BlockTransactionId: order.BlockTransactionId,
		Status:             mdb.StatusPaySuccess,
	}
	signature, err := sign.Get(orderResp, config.GetApiAuthToken())
	if err != nil {
		return err
	}
	orderResp.Signature = signature

	// 构造回调 JSON 后，发请求前
	payloadBytes, _ := json.Cjson.Marshal(orderResp)
	log.Sugar.Infof(
		"↗️ Epusdt 回调发起 | trade_id=%s | order_id=%s | url=%s | payload=%s",
		order.TradeId, order.OrderId, order.NotifyUrl, string(payloadBytes),
	)

	// 发起回调
	resp, err := client.R().
		SetHeader("powered-by", "Epusdt(https://github.com/assimon/epusdt)").
		SetBody(orderResp).
		Post(order.NotifyUrl)

	if err != nil {
		log.Sugar.Errorf("❌ Epusdt 回调请求失败 | trade_id=%s | order_id=%s | err=%v", order.TradeId, order.OrderId, err)
		return err
	}

	// 记录 HTTP 返回
	log.Sugar.Infof(
		"✅ Epusdt 回调响应 | trade_id=%s | order_id=%s | status=%d | body=%s",
		order.TradeId, order.OrderId, resp.StatusCode(), string(resp.Body()),
	)

	body := string(resp.Body())
	if body != "ok" || resp.StatusCode() != 200 {
		log.Sugar.Errorf("❌ Epusdt 回调未确认 | trade_id=%s | order_id=%s | status=%d | body=%s",
			order.TradeId, order.OrderId, resp.StatusCode(), string(resp.Body()))
		order.CallBackConfirm = mdb.CallBackConfirmNo
		return errors.New("not ok")
	}

	// 记录对账日志：包含 trade_id / order_id 与回调 payload
	log.Sugar.Infof(
		"📒 Epusdt callback record | trade_id=%s | order_id=%s | payload=%s",
		order.TradeId, order.OrderId, string(payloadBytes),
	)

	order.CallBackConfirm = mdb.CallBackConfirmOk
	return nil
}

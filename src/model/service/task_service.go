package service

import (
	"fmt"
	"github.com/assimon/luuu/model/data"
	"github.com/assimon/luuu/model/request"
	"github.com/assimon/luuu/mq"
	"github.com/assimon/luuu/mq/handle"
	"github.com/assimon/luuu/telegram"
	"github.com/assimon/luuu/util/http_client"
	"github.com/assimon/luuu/util/json"
	"github.com/assimon/luuu/util/log"
	"github.com/golang-module/carbon/v2"
	"github.com/gookit/goutil/stdutil"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"net/http"
	"sync"
	"github.com/assimon/luuu/config"
)

type UsdtTrc20Resp struct {
	PageSize int    `json:"page_size"`
	Code     int    `json:"code"`
	Data     []Data `json:"data"`
}

type TokenInfo struct {
	TokenID      string `json:"tokenId"`
	TokenAbbr    string `json:"tokenAbbr"`
	TokenName    string `json:"tokenName"`
	TokenDecimal int    `json:"tokenDecimal"`
	TokenCanShow int    `json:"tokenCanShow"`
	TokenType    string `json:"tokenType"`
	TokenLogo    string `json:"tokenLogo"`
	TokenLevel   string `json:"tokenLevel"`
	IssuerAddr   string `json:"issuerAddr"`
	Vip          bool   `json:"vip"`
}

type Data struct {
	Amount         string `json:"amount"`
	ApprovalAmount string `json:"approval_amount"`
	BlockTimestamp int64  `json:"block_timestamp"`
	Block          int    `json:"block"`
	From           string `json:"from"`
	To             string `json:"to"`
	Hash           string `json:"hash"`
	Confirmed      int    `json:"confirmed"`
	ContractType   string `json:"contract_type"`
	ContracTType   int    `json:"contractType"`
	Revert         int    `json:"revert"`
	ContractRet    string `json:"contract_ret"`
	EventType      string `json:"event_type"`
	IssueAddress   string `json:"issue_address"`
	Decimals       int    `json:"decimals"`
	TokenName      string `json:"token_name"`
	ID             string `json:"id"`
	Direction      int    `json:"direction"`
}

// Trc20CallBack trc20回调
func Trc20CallBack(token string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if err := recover(); err != nil {
			log.Sugar.Error("轮询异常:", err)
		}
	}()

	// =============== 📡 监听配置信息 ===============
	apiUri := config.GetUsdtTrc20ApiUri()
	contractId := config.GetUsdtTrc20ContractId()
	startTime := carbon.Now().AddHours(-24).TimestampWithMillisecond()
	endTime := carbon.Now().TimestampWithMillisecond()

	log.Sugar.Infof(
		"🔍 开始轮询 TRC20 转账 | API: %s | Contract: %s | 钱包地址: %s | 时间范围: [%d ~ %d]",
		apiUri, contractId, token, startTime, endTime,
	)

	// =============== 🌐 发送 API 请求 ===============
	client := http_client.GetHttpClient()
	resp, err := client.R().SetQueryParams(map[string]string{
		"sort":            "-timestamp",
		"limit":           "50",
		"start":           "0",
		"direction":       "2",
		"db_version":      "1",
		"trc20Id":         contractId,
		"address":         token,
		"start_timestamp": stdutil.ToString(startTime),
		"end_timestamp":   stdutil.ToString(endTime),
	}).Get(apiUri)

	if err != nil {
		log.Sugar.Errorf("❌ API 请求失败 [%s]: %v", token, err)
		panic(err)
	}

	if resp.StatusCode() != http.StatusOK {
		log.Sugar.Errorf("❌ API 返回异常状态码 [%s]: %d | 响应体: %s", token, resp.StatusCode(), string(resp.Body()))
		panic(err)
	}

	// =============== 📦 解析 API 响应 ===============
	var trc20Resp UsdtTrc20Resp
	err = json.Cjson.Unmarshal(resp.Body(), &trc20Resp)
	if err != nil {
		log.Sugar.Errorf("❌ 响应解析失败 [%s]: %v | 原始响应: %s", token, err, string(resp.Body()))
		panic(err)
	}

	log.Sugar.Infof("📋 API 返回 %d 条转账记录 [%s]", trc20Resp.PageSize, token)

	if trc20Resp.PageSize <= 0 {
		log.Sugar.Debugf("⚠️  没有新的转账记录 [%s]", token)
		return
	}

	// =============== 🔄 逐条处理转账记录 ===============
	for idx, transfer := range trc20Resp.Data {
		log.Sugar.Debugf(
			"[%d] 原始记录: 发送方=%s | 接收方=%s | 合约状态=%s | 金额(最小单位)=%s | 交易哈希=%s",
			idx, transfer.From, transfer.To, transfer.ContractRet, transfer.Amount, transfer.Hash,
		)

		// 过滤：只关心发送到目标钱包且合约执行成功的转账
		if transfer.To != token || transfer.ContractRet != "SUCCESS" {
			log.Sugar.Debugf(
				"⏭️  [%d] 跳过转账: 接收方不匹配或合约失败 (接收方=%s, 状态=%s)",
				idx, transfer.To, transfer.ContractRet,
			)
			continue
		}

		// =============== 💰 金额单位转换 ===============
		decimalQuant, err := decimal.NewFromString(transfer.Amount)
		if err != nil {
			log.Sugar.Errorf("❌ [%d] 金额转换失败 [%s]: %v | 原始值=%s", idx, token, err, transfer.Amount)
			continue  // ← 改为 continue，不中断轮询
		}

		// USDT 有 6 位小数，API 返回的是最小单位
		decimalDivisor := decimal.NewFromInt(1000000)
		decimalAmount := decimalQuant.Div(decimalDivisor)
		
		// ✅ 使用 StringFixed 保证精度一致（与订单创建时相同）
		amount := decimalAmount.InexactFloat64()
		amountStr := decimalAmount.StringFixed(4)  // 保留 4 位小数
		
		log.Sugar.Infof(
			"💳 [%d] 转账金额转换: %s → %s USDT [%s]",
			idx, transfer.Amount, amountStr, token,
		)

		// =============== 🔎 查询订单 ===============
		tradeId, err := data.GetTradeIdByWalletAddressAndAmount(token, amount)
		if err != nil {
			log.Sugar.Errorf("❌ [%d] 查询订单失败 [%s | %.4f]: %v", idx, token, amount, err)
			panic(err)
		}

		if tradeId == "" {
			log.Sugar.Warnf(
				"⚠️  [%d] 未找到订单 [钱包=%s | 金额=%.4f USDT] | Redis key: wallet:%s_%v | 可能原因: 1)金额不匹配 2)订单已过期 3)金额小数位处理",
				idx, token, amount, token, amount,
			)
			continue
		}

		log.Sugar.Infof("✅ [%d] 订单查询成功 | TradeId=%s | 钱包=%s | 金额=%.4f USDT", idx, tradeId, token, amount)

		// =============== 📊 获取订单详情 ===============
		order, err := data.GetOrderInfoByTradeId(tradeId)
		if err != nil {
			log.Sugar.Errorf("❌ [%d] 获取订单详情失败 [TradeId=%s]: %v", idx, tradeId, err)
			panic(err)
		}

		// =============== ⏰ 时间戳验证 ===============
		createTime := order.CreatedAt.TimestampWithMillisecond()
		log.Sugar.Debugf(
			"🕐 时间验证 | 区块时间=%d | 订单创建时间=%d | 验证结果=%v",
			transfer.BlockTimestamp, createTime, transfer.BlockTimestamp >= createTime,
		)

		if transfer.BlockTimestamp < createTime {
			log.Sugar.Errorf("❌ [%d] 时间戳验证失败 | 区块时间(%d) < 订单创建时间(%d)", idx, transfer.BlockTimestamp, createTime)
			panic("Orders cannot actually be matched")
		}

		// =============== ✨ 订单处理 ===============
		log.Sugar.Infof("🎯 [%d] 订单匹配成功，准备处理 | TradeId=%s | OrderId=%s | 金额=%.4f USDT", idx, tradeId, order.OrderId, amount)

		req := &request.OrderProcessingRequest{
			Token:              token,
			TradeId:            tradeId,
			Amount:             amount,
			BlockTransactionId: transfer.Hash,
		}

		err = OrderProcessing(req)
		if err != nil {
			log.Sugar.Errorf("❌ [%d] 订单处理失败 [TradeId=%s]: %v", idx, tradeId, err)
			panic(err)
		}

		log.Sugar.Infof("✅ [%d] 订单处理成功 | TradeId=%s | 交易哈希=%s", idx, tradeId, transfer.Hash)

		// =============== 📨 发送回调队列 ===============
		orderCallbackQueue, _ := handle.NewOrderCallbackQueue(order)
		mq.MClient.Enqueue(orderCallbackQueue, asynq.MaxRetry(5))
		log.Sugar.Infof("📤 [%d] 已入队回调任务 | TradeId=%s", idx, tradeId)

		// =============== 🤖 发送 Telegram 机器人消息 ===============
		msgTpl := `
<b>📢📢有新的交易支付成功！</b>
<pre>交易号：%s</pre>
<pre>订单号：%s</pre>
<pre>请求支付金额：%f</pre>
<pre>实际支付金额：%f usdt</pre>
<pre>钱包地址：%s</pre>
<pre>订单创建时间：%s</pre>
<pre>支付成功时间：%s</pre>
<pre>交易哈希：%s</pre>
`
		msg := fmt.Sprintf(msgTpl, order.TradeId, order.OrderId, order.Amount, order.ActualAmount, order.Token, order.CreatedAt.ToDateTimeString(), carbon.Now().ToDateTimeString(), transfer.Hash)
		telegram.SendToBot(msg)
		log.Sugar.Infof("🤖 [%d] Telegram 消息已发送 | TradeId=%s", idx, tradeId)
	}

	log.Sugar.Infof("✨ 轮询完成 [%s] | 共处理 %d 条记录", token, trc20Resp.PageSize)
}

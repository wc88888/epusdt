package task

import (
	"github.com/assimon/luuu/util/log"
	"github.com/robfig/cron/v3"
)

func Start() {
	c := cron.New()
	
	// 汇率监听
	c.AddJob("@every 60s", UsdtRateJob{})
	log.Sugar.Infof("⏱️  已注册定时任务: USDT 汇率更新 (60秒执行一次)")
	
	// trc20钱包监听
	c.AddJob("@every 5s", ListenTrc20Job{})
	log.Sugar.Infof("⏱️  已注册定时任务: TRC20 转账监听 (5秒执行一次)")
	
	c.Start()
	log.Sugar.Infof("✅ 定时任务引擎已启动")
}

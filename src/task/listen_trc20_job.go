package task

import (
	"github.com/assimon/luuu/model/data"
	"github.com/assimon/luuu/model/service"
	"github.com/assimon/luuu/util/log"
	"sync"
)

type ListenTrc20Job struct {
}

var gListenTrc20JobLock sync.Mutex

func (r ListenTrc20Job) Run() {
	gListenTrc20JobLock.Lock()
	defer gListenTrc20JobLock.Unlock()

	// =============== 📋 获取可用钱包地址 ===============
	walletAddress, err := data.GetAvailableWalletAddress()
	if err != nil {
		log.Sugar.Errorf("❌ 获取钱包地址列表失败: %v", err)
		return
	}

	if len(walletAddress) <= 0 {
		log.Sugar.Warnf("⚠️  没有可用的钱包地址进行轮询")
		return
	}

	log.Sugar.Infof("🔄 启动 TRC20 转账监听 | 轮询周期: 5秒 | 待监听钱包数: %d", len(walletAddress))

	// =============== 🔍 并发轮询各个钱包 ===============
	var wg sync.WaitGroup
	for i, address := range walletAddress {
		log.Sugar.Debugf("[%d/%d] 启动轮询任务 | 钱包地址: %s", i+1, len(walletAddress), address.Token)
		wg.Add(1)
		go service.Trc20CallBack(address.Token, &wg)
	}

	// 等待所有轮询任务完成
	wg.Wait()
	log.Sugar.Debugf("✅ 本轮轮询全部完成 | 已处理钱包数: %d", len(walletAddress))
}

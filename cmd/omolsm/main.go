package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"omolsm/config"
)

func main() {
	// 命令行参数: 指定配置文件路径
	cfgPath := flag.String("config", "config/default.yaml", "配置文件路径")
	flag.Parse()

	// 1. 加载配置
	cfg, err := config.NewConfigFromYAML(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	fmt.Println("========== OmoLSM ==========")
	fmt.Printf("存储目录:        %s\n", cfg.Dir)
	fmt.Printf("LSM Tree 层数:   %d\n", cfg.MaxLevel)
	fmt.Printf("每层 SST 上限:   %d\n", cfg.SSTNumPerLevel)
	fmt.Printf("SST 大小阈值:    %d bytes (%.1f MB)\n", cfg.SSTSize, float64(cfg.SSTSize)/(1024*1024))
	fmt.Printf("Data Block 大小: %d bytes (%.1f KB)\n", cfg.SSTDataBlockSize, float64(cfg.SSTDataBlockSize)/1024)
	fmt.Println("============================")

	// 2. 初始化 LSM Tree
	// tree, err := tree.NewTree(cfg)
	// if err != nil {
	//     log.Fatalf("初始化 LSM Tree 失败: %v", err)
	// }
	// defer tree.Close()

	// 3. 你的业务逻辑...

	os.Exit(0)
}

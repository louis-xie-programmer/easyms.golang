// Package config 提供了统一的配置管理功能。
package config

import (
	"easyms/internal/shared/models"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dario.cat/mergo"
	"github.com/fsnotify/fsnotify"
	"github.com/go-playground/validator/v10" // 引入 validator 用于结构体验证
	"gopkg.in/yaml.v2"
)

// validate 是一个单例的验证器实例。
var validate *validator.Validate

func init() {
	validate = validator.New()
}

// LocalConfig 实现了 AppConfigProvider 接口，用于从本地文件系统加载配置。
// 它支持分层加载（共享配置 + 服务特定配置）和文件变更时的热重载。
type LocalConfig struct {
	ServerName       string
	Env              string
	onChangeCallback func(*models.AppConfig)
	watcher          *fsnotify.Watcher
	mu               sync.Mutex // 用于保护 watcher 的初始化过程，防止并发问题
}

// NewLocalConfig 创建一个新的本地配置提供者。
//
// serviceName: 当前服务的名称 (例如, "user-svc", "gateway")，用于定位服务特定的配置文件。
// env: 当前运行的环境 (例如, "dev", "prod")，用于加载对应环境的配置。
func NewLocalConfig(serviceName string, env string) AppConfigProvider {
	if serviceName == "" {
		serviceName = "gateway" // 默认服务名，增加代码健壮性
	}
	if env == "" {
		env = "dev" // 默认环境，增加代码健壮性
	}
	return &LocalConfig{
		ServerName: serviceName,
		Env:        env,
	}
}

// OnChange 返回一个空操作的回调函数。
// 在 LocalConfig 中，热重载的逻辑是在内部通过 fsnotify 实现的，
// 如果需要对外通知，可以通过 SetOnChangeCallback 方法注册回调。
func (lc *LocalConfig) OnChange() func(*models.AppConfig) {
	return func(newConfig *models.AppConfig) {}
}

// SetOnChangeCallback 允许外部调用者注册一个当配置发生变更时触发的回调函数。
func (lc *LocalConfig) SetOnChangeCallback(callback func(*models.AppConfig)) {
	lc.onChangeCallback = callback
}

// LoadAppConfig 是配置加载的入口点。
// 它首先加载并合并配置，然后对合并后的配置进行验证，
// 验证通过后将其设置为全局配置，并启动文件监控以支持热重载。
func (lc *LocalConfig) LoadAppConfig() error {
	cfg, err := lc.loadAndMerge()
	if err != nil {
		return err
	}

	// 对加载和合并后的配置进行结构化验证
	if err := validate.Struct(cfg); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	// 将最终配置设置为全局应用配置
	SetAppConfig(cfg)

	// 启动文件监控
	sharedPath := GetLocalAppConfigFileName(lc.Env)
	servicePath := GetLocalServerConfigFileName(lc.ServerName, lc.Env)
	lc.startWatcher(sharedPath, servicePath)

	return nil
}

// loadAndMerge 负责实际的配置加载和合并逻辑。
// 加载顺序:
// 1. 读取共享配置文件 (例如, configs/share/dev.yaml) 作为基础。
// 2. 读取服务特定配置文件 (例如, configs/user-svc/dev.yaml)。
// 3. 将服务特定配置覆盖到共享配置之上。
func (lc *LocalConfig) loadAndMerge() (*models.AppConfig, error) {
	// 1. 加载共享配置
	sharedPath := GetLocalAppConfigFileName(lc.Env)
	sharedData, err := os.ReadFile(sharedPath)
	if err != nil {
		return nil, fmt.Errorf("读取共享配置文件 %s 失败: %w", sharedPath, err)
	}

	var finalCfg models.AppConfig
	if err := yaml.Unmarshal(sharedData, &finalCfg); err != nil {
		return nil, fmt.Errorf("解析共享配置失败: %w", err)
	}

	// 2. 加载服务特定配置
	servicePath := GetLocalServerConfigFileName(lc.ServerName, lc.Env)
	serviceData, err := os.ReadFile(servicePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果服务特定配置文件不存在，则仅使用共享配置，这是正常情况
			return &finalCfg, nil
		}
		return nil, fmt.Errorf("读取服务配置文件 %s 失败: %w", servicePath, err)
	}

	var serviceCfg models.AppConfig
	if err := yaml.Unmarshal(serviceData, &serviceCfg); err != nil {
		return nil, fmt.Errorf("解析服务配置失败: %w", err)
	}

	// 3. 合并配置，服务特定配置会覆盖共享配置中的同名顶级字段
	if err := mergo.Merge(&finalCfg, &serviceCfg, mergo.WithOverride); err != nil {
		return nil, fmt.Errorf("合并配置失败: %w", err)
	}

	return &finalCfg, nil
}

// startWatcher 初始化并启动一个文件系统观察者，用于监控配置文件的变更。
func (lc *LocalConfig) startWatcher(sharedPath, servicePath string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.watcher != nil {
		return // 观察者已在运行
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("创建配置观察者失败: %v", err)
		return
	}
	lc.watcher = watcher

	go func() {
		// 使用防抖动（Debounce）机制来避免短时间内因多次文件保存而触发多次重载
		var timer *time.Timer
		debounceDuration := 100 * time.Millisecond

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// 我们只关心写入或创建事件
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					// 重置防抖计时器
					if timer != nil {
						timer.Stop()
					}
					timer = time.AfterFunc(debounceDuration, func() {
						log.Printf("配置文件被修改: %s, 正在重新加载...", event.Name)
						if err := lc.reloadConfig(); err != nil {
							log.Printf("重新加载配置失败: %v", err)
						} else {
							log.Println("配置重新加载成功")
							// 如果设置了回调，则触发它
							if lc.onChangeCallback != nil {
								lc.onChangeCallback(GetAppConfig())
							}
						}
					})
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("配置观察者错误: %v", err)
			}
		}
	}()

	// 将配置文件路径添加到观察者
	paths := []string{sharedPath}
	if servicePath != "" {
		// 检查文件是否存在，避免为不存在的文件添加观察者
		if _, err := os.Stat(servicePath); err == nil {
			paths = append(paths, servicePath)
		}
	}

	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			log.Printf("获取绝对路径失败 %s: %v", p, err)
			continue
		}
		if err := watcher.Add(absPath); err != nil {
			log.Printf("监控配置文件 %s 失败: %v", absPath, err)
		} else {
			log.Printf("正在监控配置文件: %s", absPath)
		}
	}
}

// reloadConfig 是热重载时调用的内部方法。
// 它重新加载和合并配置，验证后设置为全局配置。
func (lc *LocalConfig) reloadConfig() error {
	cfg, err := lc.loadAndMerge()
	if err != nil {
		return err
	}

	if err := validate.Struct(cfg); err != nil {
		return fmt.Errorf("重新加载的配置验证失败: %w", err)
	}

	SetAppConfig(cfg)
	return nil
}

// GetLocalServerConfigFileName 生成服务特定配置文件的路径。
func GetLocalServerConfigFileName(server string, env string) string {
	return fmt.Sprintf("configs/%s/%s.yaml", server, env)
}

// GetLocalAppConfigFileName 生成共享配置文件的路径。
func GetLocalAppConfigFileName(env string) string {
	return fmt.Sprintf("configs/share/%s.yaml", env)
}

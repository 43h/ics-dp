package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gin-gonic/gin"
)

// 简化的登录配置结构
type Config struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LoginURL string `json:"login_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// 浏览器登录结果
type BrowserLoginResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	URL       string `json:"url"`
	PageTitle string `json:"page_title"`
	Error     string `json:"error,omitempty"`
}

// 全局配置（示例数据）
var configs = []Config{
	{
		ID:       1,
		Name:     "测试配置",
		LoginURL: "https://192.168.11.150/login",
		Username: "sysadmin",
		Password: "csmp@CLOUD987654",
	},
}

// 使用chromedp打开并显示登录页面
func openLoginPageWithChromedp(loginURL, username, password string) (*BrowserLoginResult, error) {
	fmt.Printf("使用chromedp打开登录页面: %s\n", loginURL)

	// 创建chrome浏览器选项
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),                                // 显示浏览器窗口
		chromedp.Flag("disable-gpu", false),                             // 启用GPU
		chromedp.Flag("disable-web-security", true),                     // 禁用网络安全检查
		chromedp.Flag("ignore-certificate-errors", true),                // 忽略证书错误
		chromedp.Flag("window-size", "1280,720"),                        // 设置窗口大小
		chromedp.Flag("start-maximized", false),                         // 不最大化窗口
		chromedp.Flag("disable-blink-features", "AutomationControlled"), // 隐藏自动化标识
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"),
	)

	// 创建浏览器上下文
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// 创建标签页上下文
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// 设置超时时间
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var pageTitle string
	var currentURL string
	var loginSuccess bool

	// 执行浏览器操作
	err := chromedp.Run(ctx,
		// 1. 导航到登录页面
		chromedp.Navigate(loginURL),

		// 2. 等待页面完全加载完成
		chromedp.WaitReady("body", chromedp.ByQuery),

		// 等待页面动态内容加载完成
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Printf("等待页面完全加载...\n")

			// 等待基本元素加载
			chromedp.Sleep(3 * time.Second).Do(ctx)

			// 检查是否有Angular应用
			var hasAngular bool
			err := chromedp.EvaluateAsDevTools(`
				(function() {
					return window.angular !== undefined || 
						   document.querySelector('[ng-app]') !== null ||
						   document.querySelector('[data-ng-app]') !== null ||
						   document.querySelectorAll('[_ngcontent]').length > 0;
				})()
			`, &hasAngular).Do(ctx)

			if err == nil && hasAngular {
				fmt.Printf("检测到Angular应用，等待Angular加载完成...\n")

				// 轮询等待Angular加载完成
				for i := 0; i < 10; i++ {
					var ready bool
					err := chromedp.EvaluateAsDevTools(`
						(function() {
							// 检查Angular是否完成初始化
							if (window.angular) {
								var element = document.querySelector('[ng-app], [data-ng-app]') || document.body;
								try {
									var scope = window.angular.element(element).scope();
									return scope && scope.$$phase === null;
								} catch(e) {
									return false;
								}
							}
							
							// 检查是否有pending的HTTP请求
							var pendingRequests = document.querySelectorAll('.loading, [aria-busy="true"]').length;
							if (pendingRequests > 0) {
								return false;
							}
							
							// 检查表单元素是否已渲染
							var inputs = document.querySelectorAll('input[type="text"], input[type="password"]');
							return inputs.length > 0;
						})()
					`, &ready).Do(ctx)

					if err == nil && ready {
						fmt.Printf("Angular加载完成\n")
						break
					}

					chromedp.Sleep(1 * time.Second).Do(ctx)
				}
			}

			// 检查是否有React应用
			var hasReact bool
			err = chromedp.EvaluateAsDevTools(`
				(function() {
					return window.React !== undefined || 
						   document.querySelector('[data-reactroot]') !== null ||
						   document.querySelectorAll('[data-react]').length > 0;
				})()
			`, &hasReact).Do(ctx)

			if err == nil && hasReact {
				fmt.Printf("检测到React应用，等待React渲染完成...\n")
				chromedp.Sleep(2 * time.Second).Do(ctx)
			}

			// 通用的DOM稳定性检查
			fmt.Printf("等待DOM稳定...\n")
			for i := 0; i < 10; i++ {
				var domStable bool
				err := chromedp.EvaluateAsDevTools(`
					(function() {
						// 检查是否有loading指示器
						var loadingElements = document.querySelectorAll('.loading, .spinner, [class*="loading"], [class*="spinner"]');
						if (loadingElements.length > 0) {
							for (var i = 0; i < loadingElements.length; i++) {
								var style = window.getComputedStyle(loadingElements[i]);
								if (style.display !== 'none' && style.visibility !== 'hidden') {
									return false;
								}
							}
						}
						
						// 检查输入框是否已经渲染并可见
						var visibleInputs = 0;
						var allInputs = document.querySelectorAll('input');
						for (var i = 0; i < allInputs.length; i++) {
							var input = allInputs[i];
							if (input.id == 'username') || ) {
								visibleInputs++;
							} else if (input.id == 'password') {
								visibleInputs++;
							}
						}
						return visibleInputs >= 2; // 至少有2个可见输入框
					})()
				`, &domStable).Do(ctx)

				if err == nil && domStable {
					fmt.Printf("DOM已稳定\n")
					break
				}

				chromedp.Sleep(1 * time.Second).Do(ctx)
			}

			fmt.Printf("页面加载检查完成\n")
			return nil
		}),

		// 额外等待确保所有异步操作完成
		chromedp.Sleep(1*time.Second),

		// 3. 获取页面信息
		chromedp.Title(&pageTitle),
		chromedp.Location(&currentURL),

		// 4. 显示页面信息
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Printf("=== 页面信息 ===\n")
			fmt.Printf("页面标题: %s\n", pageTitle)
			fmt.Printf("当前URL: %s\n", currentURL)

			// 获取页面内容预览
			var bodyText string
			err := chromedp.Text("body", &bodyText, chromedp.ByQuery).Do(ctx)
			if err == nil {
				if len(bodyText) > 300 {
					fmt.Printf("页面内容预览: %s...\n", bodyText[:300])
				} else {
					fmt.Printf("页面内容: %s\n", bodyText)
				}
			}

			fmt.Printf("================\n")
			return nil
		}),

		// 5. 如果提供了用户名和密码，尝试自动登录
		chromedp.ActionFunc(func(ctx context.Context) error {
			if username == "" || password == "" {
				fmt.Printf("未提供用户名或密码，跳过自动登录\n")
				return nil
			}

			fmt.Printf("尝试自动填写登录信息...\n")
			return performAutoLogin(ctx, username, password)
		}),

		// 6. 等待用户查看或操作
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Printf("页面将保持打开状态30秒，您可以手动操作...\n")
			return nil
		}),
		chromedp.Sleep(30*time.Second),

		// 7. 最终检查登录状态
		chromedp.ActionFunc(func(ctx context.Context) error {
			var finalURL string
			err := chromedp.Location(&finalURL).Do(ctx)
			if err == nil && finalURL != currentURL {
				fmt.Printf("URL已变化: %s -> %s\n", currentURL, finalURL)
				loginSuccess = true
				currentURL = finalURL
			}
			return nil
		}),
	)

	if err != nil {
		log.Printf("chromedp操作失败: %v", err)
		return &BrowserLoginResult{
			Success: false,
			Message: "打开页面失败",
			URL:     loginURL,
			Error:   fmt.Sprintf("chromedp操作失败: %v", err),
		}, err
	}

	result := &BrowserLoginResult{
		Success:   loginSuccess,
		URL:       currentURL,
		PageTitle: pageTitle,
	}

	if loginSuccess {
		result.Message = "页面打开成功，可能已登录"
	} else {
		result.Message = "页面打开成功，显示中"
	}

	fmt.Printf("chromedp操作完成: %s\n", result.Message)
	return result, nil
}

// 执行自动登录
func performAutoLogin(ctx context.Context, username, password string) error {
	// 检测页面中的图标元素
	fmt.Printf("检测页面中的用户名和密码图标...\n")

	// 详细检测页面结构
	var pageStructure string
	err := chromedp.EvaluateAsDevTools(`
		(function() {
			var structure = '';
			
			// 检查用户名相关元素
			var usernameInputs = document.querySelectorAll('input[name="username"], input[id="username"], input[placeholder*="账号"], input[placeholder*="用户"]');
			if (usernameInputs.length > 0) {
				structure += '用户名输入框信息:\n';
				usernameInputs.forEach(function(input, index) {
					structure += '  [' + index + '] ID: ' + (input.id || '无') + 
								', Name: ' + (input.name || '无') + 
								', Type: ' + (input.type || '无') + 
								', Placeholder: ' + (input.placeholder || '无') + 
								', Class: ' + (input.className || '无') + '\n';
					
					// 检查父容器
					var parent = input.parentElement;
					if (parent) {
						structure += '      父容器Class: ' + (parent.className || '无') + '\n';
					}
				});
			}
			
			// 检查密码相关元素
			var passwordInputs = document.querySelectorAll('input[type="password"], input[name="password"], input[id="password"]');
			if (passwordInputs.length > 0) {
				structure += '密码输入框信息:\n';
				passwordInputs.forEach(function(input, index) {
					structure += '  [' + index + '] ID: ' + (input.id || '无') + 
								', Name: ' + (input.name || '无') + 
								', Type: ' + (input.type || '无') + 
								', Placeholder: ' + (input.placeholder || '无') + 
								', Class: ' + (input.className || '无') + '\n';
					
					// 检查父容器
					var parent = input.parentElement;
					if (parent) {
						structure += '      父容器Class: ' + (parent.className || '无') + '\n';
					}
				});
			}
			
			// 检查登录按钮
			var buttons = document.querySelectorAll('button, input[type="submit"]');
			if (buttons.length > 0) {
				structure += '按钮信息:\n';
				buttons.forEach(function(btn, index) {
					if (btn.textContent.includes('立即登录') || btn.textContent.includes('Login') || 
						btn.value && (btn.value.includes('立即登录') || btn.value.includes('Login'))) {
						structure += '  [' + index + '] 文本: ' + (btn.textContent || btn.value || '无') + 
									', Type: ' + (btn.type || '无') + 
									', Class: ' + (btn.className || '无') + '\n';
					}
				});
			}
			
			return structure;
		})()
	`, &pageStructure).Do(ctx)

	if err == nil && pageStructure != "" {
		fmt.Printf("=== 页面结构分析 ===\n")
		fmt.Printf("%s", pageStructure)
		fmt.Printf("===================\n")
	}

	// 检测用户名图标
	var hasUsernameIcon bool
	err = chromedp.EvaluateAsDevTools(`
		(function() {
			// 检查是否存在 username-icon 或用户管理员图标
			var usernameIcon = document.querySelector('.username-icon, .csmpicon-user-admin');
			if (usernameIcon) {
				console.log('找到用户名图标:', usernameIcon.className);
				return true;
			}
			return false;
		})()
	`, &hasUsernameIcon).Do(ctx)

	if err == nil && hasUsernameIcon {
		fmt.Printf("检测到用户名图标元素\n")
	}

	// 检测密码图标
	var hasPasswordIcon bool
	err = chromedp.EvaluateAsDevTools(`
		(function() {
			// 检查是否存在 password-icon 或密码图标
			var passwordIcon = document.querySelector('.password-icon, .csmpicon-password');
			if (passwordIcon) {
				console.log('找到密码图标:', passwordIcon.className);
				return true;
			}
			return false;
		})()
	`, &hasPasswordIcon).Do(ctx)

	if err == nil && hasPasswordIcon {
		fmt.Printf("检测到密码图标元素\n")
	}

	// 扩展的用户名输入框选择器，包含特定图标相关的选择器
	usernameSelectors := []string{
		// Angular和Ant Design相关选择器（优先）
		`input#username[name="username"]`,
		`input[id="username"]`,
		`input[name="username"][nz-input]`,
		`input[placeholder*="输入账号"]`,
		`input[placeholder*="账号"]`,
		`.input-text-c input`,
		`.input-text-c input[type="text"]`,

		// 特定图标相关的选择器
		`.username-icon + input`,
		`.username-icon ~ input`,
		`.csmpicon-user-admin + input`,
		`.csmpicon-user-admin ~ input`,
		`input[class*="username"]`,
		`input[id*="username"]`,

		// 通用选择器
		`input[name="username"]`,
		`input[name="user"]`,
		`input[name="email"]`,
		`input[name="login"]`,
		`input[name="account"]`,
		`input[type="text"]`,
		`input[type="email"]`,
		`#username`, `#user`, `#email`, `#login`, `#account`,
		`.username`, `.user`, `.login`,
		`input[placeholder*="用户"]`,
		`input[placeholder*="Username"]`,
		`input[placeholder*="User"]`,
		`input[placeholder*="Account"]`,
	}

	// 扩展的密码输入框选择器，包含特定图标相关的选择器
	passwordSelectors := []string{
		// Angular和Ant Design相关选择器（优先）
		`input#password[name="password"]`,
		`input[id="password"]`,
		`input[name="password"][nz-input]`,
		`input[placeholder*="输入密码"]`,
		`input[placeholder*="密码"]`,
		`.input-text-c input[type="password"]`,

		// 特定图标相关的选择器
		`.password-icon + input`,
		`.password-icon ~ input`,
		`.csmpicon-password + input`,
		`.csmpicon-password ~ input`,
		`input[class*="password"]`,
		`input[id*="password"]`,

		// 通用选择器
		`input[name="password"]`,
		`input[name="pwd"]`,
		`input[name="passwd"]`,
		`input[type="password"]`,
		`#password`, `#pwd`, `#passwd`,
		`.password`, `.pwd`,
		`input[placeholder*="Password"]`,
		`input[placeholder*="Pass"]`,
	}

	// 常用的登录按钮选择器
	buttonSelectors := []string{
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button:contains("立即登录")`,
		`button:contains("Login")`,
		`input[value*="立即登录"]`,
		`input[value*="Login"]`,
		`.login-btn`, `.login-button`,
		`#login`, `#loginBtn`,
		`button.btn-primary`,
		`form button`,
	}

	// 尝试填写用户名
	for _, selector := range usernameSelectors {
		var visible bool
		err := chromedp.EvaluateAsDevTools(fmt.Sprintf(`
			(function() {
				var el = document.querySelector('%s');
				if (!el) return false;
				var style = window.getComputedStyle(el);
				return style.display !== 'none' && style.visibility !== 'hidden' && 
					   el.offsetWidth > 0 && el.offsetHeight > 0;
			})()
		`, selector), &visible).Do(ctx)

		if err == nil && visible {
			fmt.Printf("找到用户名输入框: %s\n", selector)
			err = chromedp.Run(ctx,
				chromedp.Clear(selector, chromedp.ByQuery),
				chromedp.SendKeys(selector, username, chromedp.ByQuery),
			)
			if err == nil {
				fmt.Printf("成功填入用户名\n")
				break
			}
		}
	}

	chromedp.Sleep(500 * time.Millisecond).Do(ctx)

	// 尝试填写密码
	for _, selector := range passwordSelectors {
		var visible bool
		err := chromedp.EvaluateAsDevTools(fmt.Sprintf(`
			(function() {
				var el = document.querySelector('%s');
				if (!el) return false;
				var style = window.getComputedStyle(el);
				return style.display !== 'none' && style.visibility !== 'hidden' && 
					   el.offsetWidth > 0 && el.offsetHeight > 0;
			})()
		`, selector), &visible).Do(ctx)

		if err == nil && visible {
			fmt.Printf("找到密码输入框: %s\n", selector)
			err = chromedp.Run(ctx,
				chromedp.Clear(selector, chromedp.ByQuery),
				chromedp.SendKeys(selector, password, chromedp.ByQuery),
			)
			if err == nil {
				fmt.Printf("成功填入密码\n")
				break
			}
		}
	}

	chromedp.Sleep(500 * time.Millisecond).Do(ctx)

	// 尝试点击登录按钮
	for _, selector := range buttonSelectors {
		var visible bool
		err := chromedp.EvaluateAsDevTools(fmt.Sprintf(`
			(function() {
				var el = document.querySelector('%s');
				if (!el) return false;
				var style = window.getComputedStyle(el);
				return style.display !== 'none' && style.visibility !== 'hidden' && 
					   el.offsetWidth > 0 && el.offsetHeight > 0;
			})()
		`, selector), &visible).Do(ctx)

		if err == nil && visible {
			fmt.Printf("找到登录按钮: %s\n", selector)
			err = chromedp.Click(selector, chromedp.ByQuery).Do(ctx)
			if err == nil {
				fmt.Printf("成功点击登录按钮\n")
				chromedp.Sleep(2 * time.Second).Do(ctx)
				return nil
			}
		}
	}

	// 如果没找到按钮，尝试回车提交
	fmt.Printf("未找到登录按钮，尝试回车提交\n")
	return chromedp.SendKeys(`input[type="password"]`, "\r", chromedp.ByQuery).Do(ctx)
}

// 登录处理函数
func login(c *gin.Context) {
	var loginData struct {
		ConfigID   int    `json:"config_id"`
		LoginURL   string `json:"login_url"`   // 可选：直接提供URL
		Username   string `json:"username"`    // 可选：直接提供用户名
		Password   string `json:"password"`    // 可选：直接提供密码
		UseBrowser bool   `json:"use_browser"` // 是否使用浏览器自动化
	}

	if err := c.ShouldBindJSON(&loginData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var loginURL, username, password string

	// 如果直接提供了URL，使用直接提供的参数
	if loginData.LoginURL != "" {
		loginURL = loginData.LoginURL
		username = loginData.Username
		password = loginData.Password
	} else {
		// 否则从配置中查找
		var config *Config
		for _, cfg := range configs {
			if cfg.ID == loginData.ConfigID {
				config = &cfg
				break
			}
		}

		if config == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "配置未找到"})
			return
		}

		loginURL = config.LoginURL
		username = config.Username
		password = config.Password
	}

	if loginURL == "" {
		loginURL = "https://192.168.11.150/login" // 默认URL
	}

	fmt.Printf("=== 开始登录流程 ===\n")
	fmt.Printf("登录URL: %s\n", loginURL)
	fmt.Printf("使用浏览器: %v\n", loginData.UseBrowser)

	if loginData.UseBrowser {
		// 使用chromedp浏览器自动化
		result, err := openLoginPageWithChromedp(loginURL, username, password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "浏览器自动化失败",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, result)
	} else {
		// 返回简单响应（不使用浏览器）
		c.JSON(http.StatusOK, gin.H{
			"message": "请设置 use_browser: true 来使用浏览器自动化",
			"url":     loginURL,
		})
	}
}

// 主函数
func main() {
	// 创建Gin路由
	r := gin.Default()

	// 添加CORS中间件
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// API路由
	r.POST("/login", login)

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "message": "Login Service is running"})
	})

	// 获取配置列表
	r.GET("/configs", func(c *gin.Context) {
		c.JSON(200, gin.H{"configs": configs})
	})

	result, err := openLoginPageWithChromedp(configs[0].LoginURL, configs[0].Username, configs[0].Password)
	if err != nil {
		log.Fatalf("测试失败: %v", err)
	}
	fmt.Printf("测试结果: %+v\n", result)

	// 启动服务器
	fmt.Println("=== Login Service ===")
	fmt.Println("服务启动在端口 8080")
}

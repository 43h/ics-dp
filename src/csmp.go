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

// 浏览器登录结果
type BrowserLoginResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	URL       string `json:"url"`
	PageTitle string `json:"page_title"`
	Error     string `json:"error,omitempty"`
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
			for i := 0; i < 10; i++ { // 减少轮询次数到10次
				var domStable bool
				err := chromedp.EvaluateAsDevTools(`
					(function() {
						console.log('DOM稳定性检查第' + (arguments[0] || 0) + '次');
						
						// 检查是否有明显的loading指示器
						var loadingElements = document.querySelectorAll('.loading, .spinner, .ant-spin-spinning');
						var visibleLoading = 0;
						for (var i = 0; i < loadingElements.length; i++) {
							var style = window.getComputedStyle(loadingElements[i]);
							if (style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0') {
								visibleLoading++;
							}
						}
						console.log('可见loading元素数量:', visibleLoading);
						
						// 如果有明显的loading，继续等待
						if (visibleLoading > 0) {
							console.log('检测到loading状态，继续等待');
							return false;
						}
						
						// 检查输入框是否已经渲染并可见
						var visibleInputs = 0;
						var passwordInputs = 0;
						var textInputs = 0;
						
						var allInputs = document.querySelectorAll('input');
						console.log('总input元素数量:', allInputs.length);
						
						for (var i = 0; i < allInputs.length; i++) {
							var input = allInputs[i];
							var style = window.getComputedStyle(input);
							if (style.display !== 'none' && style.visibility !== 'hidden' && 
								input.offsetWidth > 0 && input.offsetHeight > 0) {
								visibleInputs++;
								if (input.type === 'password') {
									passwordInputs++;
								} else if (input.type === 'text' || input.type === 'email') {
									textInputs++;
								}
							}
						}
						
						console.log('可见输入框总数:', visibleInputs, '文本框:', textInputs, '密码框:', passwordInputs);
						
						// 检查按钮是否可见
						var visibleButtons = 0;
						var buttons = document.querySelectorAll('button, input[type="submit"]');
						for (var i = 0; i < buttons.length; i++) {
							var btn = buttons[i];
							var style = window.getComputedStyle(btn);
							if (style.display !== 'none' && style.visibility !== 'hidden' && 
								btn.offsetWidth > 0 && btn.offsetHeight > 0) {
								visibleButtons++;
							}
						}
						console.log('可见按钮数量:', visibleButtons);
						
						// 更宽松的检查条件：
						// 1. 没有明显的loading指示器 AND
						// 2. (有密码输入框 OR 有至少1个文本输入框) AND
						// 3. (有按钮 OR 有表单)
						var hasValidInputs = (passwordInputs >= 1) || (textInputs >= 1);
						var hasValidSubmit = visibleButtons >= 1 || document.querySelectorAll('form').length >= 1;
						
						var isStable = hasValidInputs && hasValidSubmit;
						console.log('DOM稳定性结果:', isStable, '输入框检查:', hasValidInputs, '提交方式检查:', hasValidSubmit);
						
						// 如果第5次检查仍未通过，但有基本的输入框，也认为稳定
						if (!isStable && arguments[0] >= 4 && visibleInputs >= 1) {
							console.log('强制认为页面稳定 - 检查次数>=5且有输入框');
							return true;
						}
						
						return isStable;
					})()
				`, &domStable).Do(ctx)

				if err == nil && domStable {
					fmt.Printf("DOM已稳定(第%d次检查)\n", i+1)
					break
				}

				fmt.Printf("DOM未稳定，继续等待...(第%d次检查)\n", i+1)
				chromedp.Sleep(1500 * time.Millisecond).Do(ctx) // 减少等待时间到1.5秒
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

			// 在自动登录前再次确认页面完全加载
			fmt.Printf("自动登录前的最终页面检查...\n")
			for i := 0; i < 3; i++ { // 减少检查次数到3次
				var finalCheck bool
				err := chromedp.EvaluateAsDevTools(`
					(function() {
						// 简化的最终检查
						var inputs = document.querySelectorAll('input');
						var visibleInputs = 0;
						
						for (var i = 0; i < inputs.length; i++) {
							var input = inputs[i];
							var style = window.getComputedStyle(input);
							// 只检查基本可见性，不检查disabled状态
							if (style.display !== 'none' && style.visibility !== 'hidden' && 
								input.offsetWidth > 0 && input.offsetHeight > 0) {
								visibleInputs++;
							}
						}
						
						// 检查是否还有明显的loading状态
						var activeLoading = document.querySelectorAll('.ant-spin-spinning, .loading:not([style*="display: none"])').length;
						
						console.log('最终检查 - 可见输入框:', visibleInputs, '活跃loading:', activeLoading);
						
						// 宽松条件：有输入框且没有明显的loading
						return visibleInputs >= 1 && activeLoading === 0;
					})()
				`, &finalCheck).Do(ctx)

				if err == nil && finalCheck {
					fmt.Printf("最终页面检查通过，开始自动登录\n")
					break
				}

				if i < 2 {
					fmt.Printf("页面仍未完全就绪，等待500ms后重试...\n")
					chromedp.Sleep(500 * time.Millisecond).Do(ctx)
				} else {
					fmt.Printf("最终检查完成，开始尝试自动登录\n")
				}
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
	fmt.Printf("快速查找并填写登录信息...\n")

	// 使用JavaScript一次性查找所有需要的元素并进行填写
	var loginResult string
	err := chromedp.EvaluateAsDevTools(`
		(function() {
			var result = {
				usernameInput: null,
				passwordInput: null,
				loginButton: null,
				success: false,
				message: ''
			};

			// 优先级选择器 - 从最具体到最通用
			var usernameSelectors = [
				'input#username[name="username"]',
				'input[id="username"]',
				'input[name="username"]',
				'.username-icon + input',
				'.csmpicon-user-admin + input',
				'input[placeholder*="账号"]',
				'input[placeholder*="用户"]',
				'input[type="text"]',
				'input[type="email"]'
			];

			var passwordSelectors = [
				'input#password[name="password"]',
				'input[id="password"]',
				'input[name="password"]',
				'input[type="password"]',
				'.password-icon + input',
				'.csmpicon-password + input'
			];

			var buttonSelectors = [
				'button[type="submit"]',
				'input[type="submit"]',
				'button:contains("立即登录")',
				'button:contains("Login")',
				'form button',
				'button.btn-primary'
			];

			// 查找用户名输入框
			for (var i = 0; i < usernameSelectors.length; i++) {
				var input = document.querySelector(usernameSelectors[i]);
				if (input && input.offsetWidth > 0 && input.offsetHeight > 0) {
					var style = window.getComputedStyle(input);
					if (style.display !== 'none' && style.visibility !== 'hidden' && !input.disabled) {
						result.usernameInput = usernameSelectors[i];
						console.log('找到用户名输入框:', usernameSelectors[i]);
						break;
					}
				}
			}

			// 查找密码输入框
			for (var i = 0; i < passwordSelectors.length; i++) {
				var input = document.querySelector(passwordSelectors[i]);
				if (input && input.offsetWidth > 0 && input.offsetHeight > 0) {
					var style = window.getComputedStyle(input);
					if (style.display !== 'none' && style.visibility !== 'hidden' && !input.disabled) {
						result.passwordInput = passwordSelectors[i];
						console.log('找到密码输入框:', passwordSelectors[i]);
						break;
					}
				}
			}

			// 查找登录按钮
			for (var i = 0; i < buttonSelectors.length; i++) {
				var btn = document.querySelector(buttonSelectors[i]);
				if (btn && btn.offsetWidth > 0 && btn.offsetHeight > 0) {
					var style = window.getComputedStyle(btn);
					if (style.display !== 'none' && style.visibility !== 'hidden' && !btn.disabled) {
						result.loginButton = buttonSelectors[i];
						console.log('找到登录按钮:', buttonSelectors[i]);
						break;
					}
				}
			}

			// 检查是否找到必要元素
			if (result.usernameInput && result.passwordInput) {
				result.success = true;
				result.message = '找到用户名和密码输入框';
			} else if (result.passwordInput) {
				result.success = true;
				result.message = '找到密码输入框';
			} else {
				result.message = '未找到合适的输入框';
			}

			return JSON.stringify(result);
		})()
	`, &loginResult).Do(ctx)

	if err != nil {
		fmt.Printf("查找元素失败: %v\n", err)
		return err
	}

	fmt.Printf("元素查找结果: %s\n", loginResult)

	fmt.Printf("元素查找结果: %s\n", loginResult)

	// 解析查找结果并直接填写
	var fillSuccess bool
	err = chromedp.EvaluateAsDevTools(fmt.Sprintf(`
		(function() {
			try {
				var loginResult = %s;
				var username = %q;
				var password = %q;
				var success = true;
				var messages = [];

				// 填写用户名
				if (loginResult.usernameInput) {
					var usernameEl = document.querySelector(loginResult.usernameInput);
					if (usernameEl) {
						usernameEl.focus();
						usernameEl.value = '';
						usernameEl.value = username;
						
						// 触发change事件
						var changeEvent = new Event('change', { bubbles: true });
						usernameEl.dispatchEvent(changeEvent);
						var inputEvent = new Event('input', { bubbles: true });
						usernameEl.dispatchEvent(inputEvent);
						
						messages.push('✓ 用户名填写成功');
					} else {
						success = false;
						messages.push('✗ 用户名输入框不可用');
					}
				}

				// 填写密码
				if (loginResult.passwordInput) {
					var passwordEl = document.querySelector(loginResult.passwordInput);
					if (passwordEl) {
						passwordEl.focus();
						passwordEl.value = '';
						passwordEl.value = password;
						
						// 触发change事件
						var changeEvent = new Event('change', { bubbles: true });
						passwordEl.dispatchEvent(changeEvent);
						var inputEvent = new Event('input', { bubbles: true });
						passwordEl.dispatchEvent(inputEvent);
						
						messages.push('✓ 密码填写成功');
					} else {
						success = false;
						messages.push('✗ 密码输入框不可用');
					}
				}

				console.log(messages.join(', '));
				return success;
			} catch (e) {
				console.error('填写失败:', e);
				return false;
			}
		})()
	`, loginResult, username, password), &fillSuccess).Do(ctx)

	if err != nil {
		fmt.Printf("填写失败: %v\n", err)
		return err
	}

	if fillSuccess {
		fmt.Printf("账号密码填写成功\n")
	} else {
		fmt.Printf("账号密码填写失败\n")
	}

	// 短暂等待
	chromedp.Sleep(300 * time.Millisecond).Do(ctx)

	// 尝试点击登录按钮
	var buttonClicked bool
	err = chromedp.EvaluateAsDevTools(fmt.Sprintf(`
		(function() {
			try {
				var loginResult = %s;
				if (loginResult.loginButton) {
					var btn = document.querySelector(loginResult.loginButton);
					if (btn && !btn.disabled) {
						btn.click();
						console.log('✓ 登录按钮点击成功');
						return true;
					}
				}
				
				// 如果没有找到按钮，尝试查找其他登录按钮
				var buttons = document.querySelectorAll('button, input[type="submit"]');
				for (var i = 0; i < buttons.length; i++) {
					var btn = buttons[i];
					var text = (btn.textContent || btn.value || '').toLowerCase();
					if (text.includes('登录') || text.includes('login')) {
						var style = window.getComputedStyle(btn);
						if (style.display !== 'none' && style.visibility !== 'hidden' && !btn.disabled) {
							btn.click();
							console.log('✓ 找到并点击登录按钮:', text);
							return true;
						}
					}
				}
				
				console.log('✗ 未找到可用的登录按钮');
				return false;
			} catch (e) {
				console.error('按钮点击失败:', e);
				return false;
			}
		})()
	`, loginResult), &buttonClicked).Do(ctx)

	if err == nil && buttonClicked {
		fmt.Printf("登录按钮点击成功\n")
		chromedp.Sleep(1 * time.Second).Do(ctx)
		return nil
	}

	// 如果按钮点击失败，尝试回车提交
	fmt.Printf("尝试使用回车键提交\n")
	return chromedp.SendKeys(`input[type="password"]`, "\r", chromedp.ByQuery).Do(ctx)
}

// 登录处理函数
func csmp(c *gin.Context) {
	var loginURL, username, password string

	id := c.Param("id")
	for _, config := range configs {
		if fmt.Sprintf("%d", config.ID) == id {
			loginURL = config.LoginURL
			username = config.Username
			password = config.Password
		}
	}

	if loginURL == "" || username == "" || password == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "登录信息错误",
			"details": "",
		})
		return
	}

	result, err := openLoginPageWithChromedp(loginURL, username, password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "浏览器自动化失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}
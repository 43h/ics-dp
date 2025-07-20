package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/gin-gonic/gin"
)

type BrowserLoginResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    string `json:"data,omitempty"` // 新增
}

func getCsmpDevPageWithChromedp(loginURL, username, password string) (*BrowserLoginResult, error) {
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
	var devicesInfo string
	var searchMenu bool

	// 执行浏览器操作
	err := chromedp.Run(ctx,
		// 1. 导航到登录页面
		chromedp.Navigate(loginURL),

		// 2. 等待"登录按键"
		chromedp.WaitReady(`//button[span[contains(text(),"立即登录")]]`, chromedp.BySearch),

		// 3. 尝试自动登录
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.Title(&pageTitle).Do(ctx)
			chromedp.Location(&currentURL).Do(ctx)
			return performAutoLogin(ctx, username, password)
		}),

		// 4. 最终检查登录状态
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.WaitVisible(`//span[contains(text(),"资源概况")]`, chromedp.BySearch).Do(ctx)
			var finalURL string
			err := chromedp.Location(&finalURL).Do(ctx)
			if err == nil && finalURL != currentURL {
				loginSuccess = true
				currentURL = finalURL
				return nil
			} else {
				return errors.New("登录失败")
			}
		}),

		// 5. 进入我的服务
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.WaitReady(`//span[@title="我的服务"]`, chromedp.BySearch).Do(ctx)
			err := chromedp.EvaluateAsDevTools(`
				(function() {
					var spans = document.querySelectorAll('span');
					for (var i = 0; i < spans.length; i++) {
						var span = spans[i];
						if (span.title === "我的服务") {
							// 找到后，向上查找可点击的父元素
							var el = span;
							while (el && el !== document.body) {
								if (el.classList && el.classList.contains('q-menu-vertical-submenu__title')) {
									el.click();
									return true;
								}
								el = el.parentElement;
							}
						}
					}
					console.log('未找到“我的服务”菜单');
					return false;
				})()
			`, &searchMenu).Do(ctx)
			if err != nil || !searchMenu {
				return errors.New("未找到“我的服务”菜单")
			} else {
				return nil
			}
		}),

		// 6. 点击组件列表
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.Sleep(500 * time.Millisecond).Do(ctx) // 等待菜单展开
			searchMenu = false
			err := chromedp.EvaluateAsDevTools(`
				 (function() {
					// 查找所有 span，找到 title="组件列表" 的
					var spans = document.querySelectorAll('span[title="组件列表"]');
					for (var i = 0; i < spans.length; i++) {
						var span = spans[i];
						if (span.title === "组件列表") {
							// 向上查找 a 标签
							var el = span;
							while (el && el !== document.body) {
								if (el.tagName && el.tagName.toLowerCase() === 'a') {
									el.click();
									return true;
								}
								el = el.parentElement;
							}
						}
					}
					
					return false;
				})()
			`, &searchMenu).Do(ctx)
			if err != nil || !searchMenu {
				return errors.New("未找到“组件列表”菜单")
			} else {
				return nil
			}
		}),

		// 7. 获取body内容
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.Sleep(2 * time.Second).Do(ctx) // 等待页面加载
			chromedp.WaitVisible(`//th[contains(text(),"组件名称")]`, chromedp.BySearch).Do(ctx)
			var finalURL string
			err := chromedp.Location(&finalURL).Do(ctx)
			if err == nil && finalURL != currentURL {
				loginSuccess = true
				currentURL = finalURL
			}

			err = chromedp.OuterHTML(".ant-table-body", &devicesInfo, chromedp.ByQuery).Do(ctx)
			if err != nil {
				return err
			} else {
				return nil
			}
		}),
	)

	//检测浏览器登录返回
	if err != nil {
		fmt.Printf("chromedp操作失败: %v\n", err)
		return &BrowserLoginResult{
			Success: false,
			Error:   err.Error(),
		}, err
	}
	result := &BrowserLoginResult{
		Success: loginSuccess,
		Data:    devicesInfo,
	}

	return result, nil
}

// 执行自动登录
func performAutoLogin(ctx context.Context, username, password string) error {
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
				'input[id="username"]'
			];

			var passwordSelectors = [
				'input[id="password"]'
			];

			var buttonSelectors = [
				'button[type="submit"]',
				'input[type="submit"]',
				'form button',
				'button.btn-primary',
				'button.ant-btn-primary'
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

func parseTableHTMLToListItems(html string) ([]ListItem, error) {
	var items []ListItem
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	// 解析表头
	var headers []string
	doc.Find("thead tr th").Each(func(i int, th *goquery.Selection) {
		headers = append(headers, strings.TrimSpace(th.Text()))
	})

	// 判断表格类型
	isLongTable := false
	for _, h := range headers {
		if strings.Contains(h, "描述信息") || strings.Contains(h, "HA角色") {
			isLongTable = true
			break
		}
	}

	// 解析数据行
	doc.Find("tbody tr").Each(func(i int, tr *goquery.Selection) {
		tds := tr.Find("td")
		if isLongTable {
			// 长表格，列数多
			if tds.Length() < 13 {
				return
			}
			item := ListItem{
				Name:          strings.TrimSpace(tds.Eq(1).Find("a").Text()),
				ComponentType: strings.TrimSpace(tds.Eq(2).Text()),
				IPAddress:     strings.TrimSpace(tds.Eq(9).Text()),
				Status:        strings.TrimSpace(tds.Eq(12).Find("span.ant-badge-status-text").Text()),
			}
			items = append(items, item)
		} else {
			// 短表格，列数少
			if tds.Length() < 9 {
				return
			}
			item := ListItem{
				Name:          strings.TrimSpace(tds.Eq(1).Find("a").Text()),
				ComponentType: strings.TrimSpace(tds.Eq(2).Text()),
				IPAddress:     strings.TrimSpace(tds.Eq(7).Text()),
				Status:        strings.TrimSpace(tds.Eq(8).Find("span.ant-badge-status-text").Text()),
			}
			items = append(items, item)
		}
	})
	return items, nil
}

// 登录处理函数
func handleCsmp(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "配置信息错误",
		})
		return
	}

	result, err := getCsmpDevPageWithChromedp(loginURL, username, password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	listItems, err := parseTableHTMLToListItems(result.Data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, listItems)
}

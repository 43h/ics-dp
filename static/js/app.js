class ICPlatform {
    constructor() {
        this.configs = [];
        this.currentConfig = null;
        this.currentData = [];
        this.sessionKeys = {};
        this.currentView = 'devices';
        this.devices = [];
        this.currentWebShellDevice = null;
        this.webShellHistory = [];
        
        this.init();
    }

    async init() {
        this.setupEventListeners();
        this.addLogToInfoPanel('系统正在初始化...', 'info');
        await this.loadConfigs();
        // 等待配置加载和自动登录完成后再加载设备
        await this.loadDevices();
        this.addLogToInfoPanel('系统初始化完成', 'success');
    }

    setupEventListeners() {
        // 菜单切换
        document.querySelectorAll('.menu-item').forEach(item => {
            item.addEventListener('click', () => {
                this.switchView(item.dataset.menu);
            });
        });

        // 添加配置按钮
        document.getElementById('add-config-btn').addEventListener('click', () => {
            this.showConfigModal();
        });

        // 刷新设备按钮
        document.getElementById('refresh-all-devices-btn').addEventListener('click', () => {
            this.loadDevices();
        });

        // 清除日志按钮
        document.getElementById('clear-logs-btn').addEventListener('click', () => {
            this.clearInfoPanel();
        });

        // SSH测试按钮
        document.getElementById('test-ssh-btn').addEventListener('click', () => {
            this.testSSHConnection();
        });

        // 配置表单提交
        document.getElementById('config-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.saveConfig();
        });

        // 模态框关闭
        document.querySelectorAll('.close-btn, #cancel-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                this.hideModals();
            });
        });

        // 命令执行模态框
        document.getElementById('close-command-btn').addEventListener('click', () => {
            this.hideModals();
        });

        document.getElementById('execute-command-btn').addEventListener('click', () => {
            this.executeCommand();
        });

        // 预设命令按钮
        document.querySelectorAll('.preset-commands .btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.getElementById('command-input').value = btn.dataset.command;
            });
        });

        // 点击模态框外部关闭
        document.querySelectorAll('.modal').forEach(modal => {
            modal.addEventListener('click', (e) => {
                if (e.target === modal) {
                    this.hideModals();
                }
            });
        });
    }

    showLoading() {
        document.getElementById('loading').style.display = 'flex';
    }

    hideLoading() {
        document.getElementById('loading').style.display = 'none';
    }

    showConfigModal(config = null) {
        const modal = document.getElementById('config-modal');
        const title = document.getElementById('modal-title');
        const form = document.getElementById('config-form');
        const sshTestResult = document.getElementById('ssh-test-result');

        // 隐藏SSH测试结果
        sshTestResult.style.display = 'none';

        if (config) {
            title.textContent = '编辑配置';
            this.fillForm(form, config);
            form.dataset.configId = config.id;
        } else {
            title.textContent = '添加配置';
            form.reset();
            delete form.dataset.configId;
        }

        modal.classList.add('show');
    }

    showCommandModal(itemId) {
        const modal = document.getElementById('command-modal');
        modal.dataset.itemId = itemId;
        
        // 重置表单
        document.getElementById('command-input').value = '';
        document.getElementById('command-output').style.display = 'none';
        
        modal.classList.add('show');
    }

    hideModals() {
        document.querySelectorAll('.modal').forEach(modal => {
            modal.classList.remove('show');
        });
    }

    fillForm(form, data) {
        Object.keys(data).forEach(key => {
            const input = form.querySelector(`[name="${key}"]`);
            if (input) {
                input.value = data[key];
            }
        });
    }

    async loadConfigs() {
        try {
            this.addLogToInfoPanel('正在加载配置文件...', 'info');
            const response = await fetch('/api/configs');
            this.configs = await response.json();
            this.renderConfigs();
            
            if (this.configs.length === 0) {
                this.addLogToInfoPanel('暂无配置，请点击"配置管理"添加配置', 'warning');
            } else {
                this.addLogToInfoPanel(`已加载 ${this.configs.length} 个配置`, 'success');
                // 自动登录所有配置
                await this.autoLoginAllConfigs();
            }
        } catch (error) {
            console.error('加载配置失败:', error);
            this.showNotification('加载配置失败', 'error');
            this.addLogToInfoPanel('加载配置失败: ' + error.message, 'error');
        }
    }

    async autoLoginAllConfigs() {
        this.addLogToInfoPanel('开始自动登录所有配置...', 'info');
        
        let successCount = 0;
        let failureCount = 0;
        
        for (const config of this.configs) {
            try {
                this.addLogToInfoPanel(`正在登录 ${config.name}...`, 'info');
                
                const response = await fetch('/api/login', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        config_id: config.id
                    })
                });

                const result = await response.json();

                if (response.ok) {
                    this.sessionKeys[config.id] = result.session_key;
                    this.addLogToInfoPanel(`${config.name} 登录成功`, 'success');
                    successCount++;
                } else {
                    throw new Error(result.error || '登录失败');
                }
            } catch (error) {
                console.error(`${config.name} 登录失败:`, error);
                this.addLogToInfoPanel(`${config.name} 登录失败: ${error.message}`, 'error');
                failureCount++;
            }
        }
        
        this.addLogToInfoPanel(`配置自动登录完成 - 成功: ${successCount}, 失败: ${failureCount}`, 'info');
        
        // 如果有成功的登录，立即更新设备状态
        if (successCount > 0) {
            this.addLogToInfoPanel('正在更新设备状态...', 'info');
        }
    }

    renderConfigs() {
        const container = document.getElementById('config-list');
        
        if (this.configs.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: #7f8c8d;">暂无配置</p>';
            return;
        }

        container.innerHTML = this.configs.map(config => `
            <div class="config-item ${this.currentConfig?.id === config.id ? 'active' : ''}" 
                 data-config-id="${config.id}">
                <h4>${config.name}</h4>
                <p>登录: ${config.login_url}</p>
                <p>数据: ${config.data_url}</p>
                <div class="config-actions">
                    <button class="btn btn-outline" onclick="app.selectConfig(${config.id})">选择</button>
                    <button class="btn btn-outline" onclick="app.editConfig(${config.id})">编辑</button>
                    <button class="btn btn-danger" onclick="app.deleteConfig(${config.id})">删除</button>
                </div>
            </div>
        `).join('');
    }

    selectConfig(configId) {
        this.currentConfig = this.configs.find(c => c.id === configId);
        if (this.currentConfig) {
            // 在新的UI中，我们只需要更新状态，不需要更新UI元素
            this.renderConfigs(); // 重新渲染以更新选中状态
            this.addLogToInfoPanel(`已选择配置: ${this.currentConfig.name}`, 'info');
        }
    }

    editConfig(configId) {
        const config = this.configs.find(c => c.id === configId);
        if (config) {
            this.showConfigModal(config);
        }
    }

    async deleteConfig(configId) {
        if (!confirm('确定要删除这个配置吗？')) {
            return;
        }

        try {
            this.showLoading();
            const response = await fetch(`/api/configs/${configId}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                await this.loadConfigs();
                if (this.currentConfig?.id === configId) {
                    this.currentConfig = null;
                    this.addLogToInfoPanel('当前配置已删除', 'warning');
                }
                this.showNotification('配置删除成功', 'success');
            } else {
                throw new Error('删除失败');
            }
        } catch (error) {
            console.error('删除配置失败:', error);
            this.showNotification('删除配置失败', 'error');
        } finally {
            this.hideLoading();
        }
    }

    async testSSHConnection() {
        const form = document.getElementById('config-form');
        const formData = new FormData(form);
        const data = Object.fromEntries(formData);
        
        // 验证必填字段
        const requiredFields = ['ssh_host', 'ssh_user', 'ssh_pass'];
        for (const field of requiredFields) {
            if (!data[field] || data[field].trim() === '') {
                this.showNotification(`请填写${field === 'ssh_host' ? 'SSH主机' : field === 'ssh_user' ? 'SSH用户' : 'SSH密码'}`, 'warning');
                return;
            }
        }

        const resultDiv = document.getElementById('ssh-test-result');
        const testBtn = document.getElementById('test-ssh-btn');
        
        try {
            // 显示测试中状态
            testBtn.disabled = true;
            testBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 测试中...';
            resultDiv.style.display = 'block';
            resultDiv.className = 'ssh-test-result testing';
            resultDiv.innerHTML = '<i class="fas fa-clock"></i> 正在测试SSH连接...';

            const response = await fetch('/api/test-ssh', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    ssh_host: data.ssh_host.trim(),
                    ssh_user: data.ssh_user.trim(),
                    ssh_pass: data.ssh_pass,
                    ssh_port: data.ssh_port || '22'
                })
            });

            const result = await response.json();

            if (result.success) {
                resultDiv.className = 'ssh-test-result success';
                resultDiv.innerHTML = `
                    <i class="fas fa-check-circle"></i> SSH连接测试成功<br>
                    <small>${result.result.trim()}</small>
                `;
                this.showNotification('SSH连接测试成功', 'success');
            } else {
                throw new Error(result.error);
            }
        } catch (error) {
            resultDiv.className = 'ssh-test-result error';
            resultDiv.innerHTML = `
                <i class="fas fa-exclamation-triangle"></i> SSH连接测试失败<br>
                <small>${error.message}</small>
            `;
            this.showNotification('SSH连接测试失败', 'error');
        } finally {
            testBtn.disabled = false;
            testBtn.innerHTML = '<i class="fas fa-plug"></i> 测试SSH连接';
        }
    }

    async saveConfig() {
        const form = document.getElementById('config-form');
        const formData = new FormData(form);
        const data = Object.fromEntries(formData);
        
        // 验证URL格式
        const urlFields = ['login_url', 'data_url'];
        for (const field of urlFields) {
            if (data[field]) {
                try {
                    // 清理URL（移除前后空格）
                    data[field] = data[field].trim();
                    
                    // 如果没有协议，添加http://
                    if (!data[field].startsWith('http://') && !data[field].startsWith('https://')) {
                        data[field] = 'http://' + data[field];
                    }
                    
                    // 验证URL格式
                    new URL(data[field]);
                } catch (error) {
                    this.showNotification(`${field === 'login_url' ? '登录' : '数据'}URL格式不正确`, 'error');
                    return;
                }
            }
        }
        
        const isEdit = form.dataset.configId;
        const url = isEdit ? `/api/configs/${form.dataset.configId}` : '/api/configs';
        const method = isEdit ? 'PUT' : 'POST';

        try {
            this.showLoading();
            const response = await fetch(url, {
                method,
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(data)
            });

            if (response.ok) {
                await this.loadConfigs();
                this.hideModals();
                this.showNotification(isEdit ? '配置更新成功' : '配置添加成功', 'success');
            } else {
                throw new Error('保存失败');
            }
        } catch (error) {
            console.error('保存配置失败:', error);
            this.showNotification('保存配置失败', 'error');
        } finally {
            this.hideLoading();
        }
    }

    async login() {
        if (!this.currentConfig) {
            this.showNotification('请先选择一个配置', 'warning');
            return;
        }

        try {
            this.showLoading();
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    config_id: this.currentConfig.id
                })
            });

            const result = await response.json();

            if (response.ok) {
                this.sessionKeys[this.currentConfig.id] = result.session_key;
                this.showNotification('登录成功', 'success');
                this.addLogToInfoPanel(`${this.currentConfig.name} 登录成功`, 'success');
                await this.refreshData();
            } else {
                throw new Error(result.error || '登录失败');
            }
        } catch (error) {
            console.error('登录失败:', error);
            this.showNotification('登录失败: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    async refreshData() {
        if (!this.currentConfig || !this.sessionKeys[this.currentConfig.id]) {
            this.showNotification('请先登录', 'warning');
            return;
        }

        try {
            this.showLoading();
            console.log('开始刷新数据，配置ID:', this.currentConfig.id);
            
            const response = await fetch(`/api/scrape/${this.currentConfig.id}`);
            console.log('响应状态:', response.status, response.statusText);
            console.log('响应头:', Object.fromEntries(response.headers.entries()));
            
            if (response.ok) {
                const responseText = await response.text();
                console.log('响应文本长度:', responseText.length);
                console.log('响应文本前500字符:', responseText.substring(0, 500));
                
                try {
                    this.currentData = JSON.parse(responseText);
                    console.log('解析的数据:', this.currentData);
                    console.log('数据项数量:', this.currentData.length);
                    
                    this.renderData();
                    this.showNotification('数据刷新成功', 'success');
                } catch (parseError) {
                    console.error('JSON解析失败:', parseError);
                    console.log('原始响应:', responseText);
                    throw new Error('响应数据格式错误');
                }
            } else if (response.status === 401) {
                this.showNotification('会话已过期，请重新登录', 'warning');
                delete this.sessionKeys[this.currentConfig.id];
                this.addLogToInfoPanel('会话已过期，请重新登录', 'warning');
            } else {
                const errorText = await response.text();
                console.error('请求失败响应:', errorText);
                throw new Error('获取数据失败: ' + response.status);
            }
        } catch (error) {
            console.error('刷新数据失败:', error);
            this.showNotification('刷新数据失败: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    renderData() {
        // 在新的UI中，数据显示在信息面板中
        if (this.currentData.length === 0) {
            this.addLogToInfoPanel('暂无数据', 'info');
            return;
        }

        this.addLogToInfoPanel(`获取到 ${this.currentData.length} 条数据`, 'success');
        this.currentData.forEach(item => {
            this.addLogToInfoPanel(`${item.title}: ${item.description}`, 'info');
        });
    }

    getStatusClass(status) {
        switch (status) {
            case '正常': return 'normal';
            case '警告': return 'warning';
            case '错误': return 'error';
            default: return 'normal';
        }
    }

    async executeCommand() {
        const modal = document.getElementById('command-modal');
        const itemId = modal.dataset.itemId;
        const command = document.getElementById('command-input').value.trim();

        if (!command) {
            this.showNotification('请输入命令', 'warning');
            return;
        }

        if (!this.currentConfig) {
            this.showNotification('请先选择配置', 'warning');
            return;
        }

        try {
            this.showLoading();
            this.addLogToInfoPanel(`执行命令: ${command}`, 'info');
            
            const response = await fetch('/api/execute', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    config_id: this.currentConfig.id,
                    item_id: itemId,
                    command: command
                })
            });

            const result = await response.json();

            if (response.ok) {
                document.getElementById('command-output').style.display = 'block';
                document.getElementById('output-text').textContent = result.result;
                this.showNotification('命令执行成功', 'success');
                this.addLogToInfoPanel(`命令执行成功: ${result.result.substring(0, 100)}${result.result.length > 100 ? '...' : ''}`, 'success');
            } else {
                throw new Error(result.error || '命令执行失败');
            }
        } catch (error) {
            console.error('命令执行失败:', error);
            this.showNotification('命令执行失败: ' + error.message, 'error');
            this.addLogToInfoPanel(`命令执行失败: ${error.message}`, 'error');
        } finally {
            this.hideLoading();
        }
    }

    showNotification(message, type = 'info') {
        // 创建通知元素
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.innerHTML = `
            <i class="fas fa-${this.getNotificationIcon(type)}"></i>
            <span>${message}</span>
        `;

        // 添加样式
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 1rem 1.5rem;
            border-radius: 8px;
            color: white;
            font-weight: 500;
            z-index: 3000;
            display: flex;
            align-items: center;
            gap: 0.5rem;
            box-shadow: 0 4px 15px rgba(0,0,0,0.2);
            transform: translateX(100%);
            transition: transform 0.3s ease;
        `;

        // 设置背景色
        switch (type) {
            case 'success':
                notification.style.background = '#28a745';
                break;
            case 'error':
                notification.style.background = '#dc3545';
                break;
            case 'warning':
                notification.style.background = '#ffc107';
                notification.style.color = '#212529';
                break;
            default:
                notification.style.background = '#17a2b8';
        }

        document.body.appendChild(notification);

        // 显示动画
        setTimeout(() => {
            notification.style.transform = 'translateX(0)';
        }, 100);

        // 自动隐藏
        setTimeout(() => {
            notification.style.transform = 'translateX(100%)';
            setTimeout(() => {
                document.body.removeChild(notification);
            }, 300);
        }, 3000);
    }

    getNotificationIcon(type) {
        switch (type) {
            case 'success': return 'check-circle';
            case 'error': return 'exclamation-circle';
            case 'warning': return 'exclamation-triangle';
            default: return 'info-circle';
        }
    }

    addLogToInfoPanel(message, type = 'info') {
        const infoPanel = document.getElementById('info-panel');
        
        // 如果还是欢迎消息，先清空
        const welcomeMsg = infoPanel.querySelector('.info-welcome');
        if (welcomeMsg) {
            welcomeMsg.remove();
        }

        const logElement = document.createElement('div');
        logElement.className = `info-log ${type}`;
        logElement.innerHTML = `
            <div class="info-log-time">${new Date().toLocaleTimeString()}</div>
            <div class="info-log-content">${message}</div>
        `;

        infoPanel.appendChild(logElement);
        
        // 滚动到底部
        infoPanel.scrollTop = infoPanel.scrollHeight;
    }

    clearInfoPanel() {
        const infoPanel = document.getElementById('info-panel');
        infoPanel.innerHTML = `
            <div class="info-welcome">
                <i class="fas fa-info-circle"></i>
                <p>这里将显示设备操作、命令执行结果和错误信息</p>
            </div>
        `;
    }

    switchView(viewName) {
        // 更新菜单状态
        document.querySelectorAll('.menu-item').forEach(item => {
            item.classList.remove('active');
        });
        document.querySelector(`[data-menu="${viewName}"]`).classList.add('active');

        // 切换视图内容
        document.querySelectorAll('.view-content').forEach(view => {
            view.classList.remove('active');
        });
        document.getElementById(`${viewName}-view`).classList.add('active');

        this.currentView = viewName;

        // 根据视图类型加载相应数据
        if (viewName === 'devices') {
            this.loadDevices();
        } else if (viewName === 'configs') {
            // 只渲染配置，不重新加载和自动登录
            this.renderConfigs();
        }
    }

    async loadDevices() {
        try {
            this.showLoading();
            
            // 模拟从所有配置的设备获取信息
            this.devices = [];
            
            for (const config of this.configs) {
                if (this.sessionKeys[config.id]) {
                    try {
                        console.log(`正在获取设备 ${config.name} 的数据...`);
                        const response = await fetch(`/api/scrape/${config.id}`);
                        console.log(`设备 ${config.name} 响应状态:`, response.status);
                        
                        if (response.ok) {
                            const responseText = await response.text();
                            console.log(`设备 ${config.name} 响应长度:`, responseText.length);
                            
                            try {
                                const data = JSON.parse(responseText);
                                console.log(`设备 ${config.name} 解析的数据:`, data);
                                console.log(`设备 ${config.name} 组件数量:`, data.length);
                                
                                this.devices.push({
                                    id: config.id,
                                    name: config.name,
                                    status: 'online',
                                    loginUrl: config.login_url,
                                    dataUrl: config.data_url,
                                    itemCount: data.length,
                                    lastUpdate: new Date().toLocaleString(),
                                    data: data
                                });
                            } catch (parseError) {
                                console.error(`设备 ${config.name} JSON解析失败:`, parseError);
                                console.log(`设备 ${config.name} 原始响应:`, responseText.substring(0, 500));
                                throw parseError;
                            }
                        } else {
                            const errorText = await response.text();
                            console.error(`设备 ${config.name} 请求失败:`, errorText);
                            throw new Error(`HTTP ${response.status}: ${errorText}`);
                        }
                    } catch (error) {
                        console.error(`设备 ${config.name} 加载失败:`, error);
                        this.devices.push({
                            id: config.id,
                            name: config.name,
                            status: 'offline',
                            loginUrl: config.login_url,
                            dataUrl: config.data_url,
                            itemCount: 0,
                            lastUpdate: '连接失败',
                            error: error.message
                        });
                    }
                } else {
                    this.devices.push({
                        id: config.id,
                        name: config.name,
                        status: 'warning',
                        loginUrl: config.login_url,
                        dataUrl: config.data_url,
                        itemCount: 0,
                        lastUpdate: '未登录',
                        needLogin: true
                    });
                }
            }
            
            this.renderDevices();
            
        } catch (error) {
            console.error('加载设备信息失败:', error);
            this.showNotification('加载设备信息失败', 'error');
        } finally {
            this.hideLoading();
        }
    }

    renderDevices() {
        const container = document.getElementById('devices-table-body');
        
        if (this.devices.length === 0) {
            container.innerHTML = '<tr><td colspan="7" style="text-align: center; color: #7f8c8d; padding: 2rem;">暂无设备配置</td></tr>';
            return;
        }

        container.innerHTML = this.devices.map(device => `
            <tr data-device-id="${device.id}">
                <td>
                    <div class="device-name-container">
                        <button class="expand-btn" onclick="app.toggleDeviceDetails(${device.id})">
                            <i class="fas fa-chevron-right"></i>
                        </button>
                        <div>
                            <div class="device-name">${device.name}</div>
                        </div>
                    </div>
                </td>
                <td>
                    <span class="device-status-badge ${device.status}">
                        ${device.status === 'online' ? '在线' : 
                          device.status === 'offline' ? '离线' : '未登录'}
                    </span>
                </td>
                <td>${device.itemCount}</td>
                <td>${device.lastUpdate}</td>
                <td>
                    <button class="btn btn-outline webshell-btn" onclick="app.openWebShell(${device.id})" title="打开WebShell">
                        <i class="fas fa-terminal"></i> WebShell
                    </button>
                </td>
                <td>
                    <button class="btn btn-info" onclick="app.jumpToLogin(${device.id})" title="跳转到原始登录页面">
                        <i class="fas fa-external-link-alt"></i> 跳转登录
                    </button>
                </td>
                <td>
                    <div class="device-actions">
                        ${device.needLogin ? 
                            `<button class="btn btn-success" onclick="app.quickLogin(${device.id})">
                                <i class="fas fa-sign-in-alt"></i> 登录
                            </button>` : ''}
                        <button class="btn btn-outline" onclick="app.selectDeviceConfig(${device.id})">
                            <i class="fas fa-eye"></i> 查看详情
                        </button>
                        ${device.status === 'online' ? 
                            `<button class="btn btn-primary" onclick="app.refreshDevice(${device.id})">
                                <i class="fas fa-sync-alt"></i> 刷新
                            </button>` : ''}
                    </div>
                </td>
            </tr>
            <tr class="device-details-row" id="details-${device.id}">
                <td colspan="7">
                    <div class="device-details-content">
                        <h4><i class="fas fa-list"></i> 设备组件信息</h4>
                        <div id="details-content-${device.id}">
                            ${device.status === 'online' && device.data ? 
                                this.renderDeviceDetails(device.data) : 
                                '<p style="color: #7f8c8d; text-align: center; padding: 1rem;">请先登录获取设备信息</p>'}
                        </div>
                    </div>
                </td>
            </tr>
        `).join('');
    }

    renderDeviceDetails(data) {
        if (!data || data.length === 0) {
            return '<p style="color: #7f8c8d; text-align: center; padding: 1rem;">暂无组件信息</p>';
        }

        return `
            <table class="device-details-table">
                <thead>
                    <tr>
                        <th>组件名称</th>
                        <th>组件类型</th>
                        <th>IP地址</th>
                        <th>状态</th>
                        <th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    ${data.map(item => `
                        <tr>
                            <td><strong>${item.title}</strong></td>
                            <td>${item.component_type || '未知类型'}</td>
                            <td>${item.ip_address || '未知IP'}</td>
                            <td>
                                <span class="item-status ${this.getStatusClass(item.status)}">
                                    ${item.status}
                                </span>
                            </td>
                            <td>
                                ${item.can_execute ? 
                                    `<button class="btn btn-primary" onclick="app.showCommandModal('${item.id}')">
                                        <i class="fas fa-terminal"></i> 执行命令
                                    </button>` : 
                                    '<span style="color: #7f8c8d;">-</span>'}
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    }

    toggleDeviceDetails(deviceId) {
        const detailsRow = document.getElementById(`details-${deviceId}`);
        const expandBtn = document.querySelector(`[data-device-id="${deviceId}"] .expand-btn`);
        
        if (detailsRow.classList.contains('show')) {
            detailsRow.classList.remove('show');
            expandBtn.classList.remove('expanded');
            expandBtn.innerHTML = '<i class="fas fa-chevron-right"></i>';
        } else {
            detailsRow.classList.add('show');
            expandBtn.classList.add('expanded');
            expandBtn.innerHTML = '<i class="fas fa-chevron-down"></i>';
        }
    }

    async openWebShell(deviceId) {
        const device = this.devices.find(d => d.id === deviceId);
        if (!device) {
            this.showNotification('设备不存在', 'error');
            return;
        }

        const config = this.configs.find(c => c.id === deviceId);
        if (!config) {
            this.showNotification('未找到设备配置', 'error');
            return;
        }

        // 检查SSH配置
        if (!config.ssh_host || !config.ssh_user || !config.ssh_pass) {
            this.showNotification('设备SSH配置不完整，请先配置SSH信息', 'warning');
            this.selectDeviceConfig(deviceId);
            return;
        }

        // 构建WebShell URL参数
        const params = new URLSearchParams({
            deviceId: config.id,
            deviceName: device.name,
            host: config.ssh_host,
            user: config.ssh_user,
            pass: config.ssh_pass,
            port: config.ssh_port || '22'
        });

        // 在新窗口中打开WebShell
        const webshellUrl = `/webshell?${params.toString()}`;
        const windowFeatures = 'width=1000,height=700,scrollbars=yes,resizable=yes,menubar=no,toolbar=no,location=no,status=no';
        
        const newWindow = window.open(webshellUrl, `webshell-${deviceId}`, windowFeatures);
        
        if (newWindow) {
            // 窗口居中显示
            const screenLeft = window.screenLeft !== undefined ? window.screenLeft : window.screenX;
            const screenTop = window.screenTop !== undefined ? window.screenTop : window.screenY;
            const width = window.innerWidth ? window.innerWidth : document.documentElement.clientWidth ? document.documentElement.clientWidth : screen.width;
            const height = window.innerHeight ? window.innerHeight : document.documentElement.clientHeight ? document.documentElement.clientHeight : screen.height;

            const left = ((width / 2) - (1000 / 2)) + screenLeft;
            const top = ((height / 2) - (700 / 2)) + screenTop;
            
            newWindow.moveTo(left, top);
            newWindow.focus();
            
            this.addLogToInfoPanel(`WebShell窗口已打开: ${device.name}`, 'success');
        } else {
            this.showNotification('无法打开WebShell窗口，请检查浏览器弹窗设置', 'error');
        }
    }

    async sendWebShellCommand() {
        // 此方法已弃用，WebShell现在在新窗口中运行
        console.log('sendWebShellCommand method is deprecated');
    }

    closeWebShell() {
        // 此方法已弃用，WebShell现在在新窗口中运行
        console.log('closeWebShell method is deprecated');
    }

    async quickLogin(configId) {
        const config = this.configs.find(c => c.id === configId);
        if (!config) return;

        this.currentConfig = config;
        await this.login();
        await this.loadDevices();
    }

    jumpToLogin(configId) {
        const config = this.configs.find(c => c.id === configId);
        if (!config) {
            this.showNotification('设备配置不存在', 'error');
            return;
        }

        if (!config.login_url) {
            this.showNotification('该设备未配置登录地址', 'warning');
            return;
        }

        // 确保URL有协议
        let loginUrl = config.login_url.trim();
        if (!loginUrl.startsWith('http://') && !loginUrl.startsWith('https://')) {
            loginUrl = 'http://' + loginUrl;
        }

        // 在新标签页打开登录页面
        window.open(loginUrl, '_blank');
        
        this.showNotification(`已在新标签页打开 ${config.name} 的登录页面`, 'info');
        this.addLogToInfoPanel(`跳转到 ${config.name} 登录页面: ${loginUrl}`, 'info');
    }

    selectDeviceConfig(configId) {
        this.switchView('configs');
        this.selectConfig(configId);
    }

    async refreshDevice(deviceId) {
        const device = this.devices.find(d => d.id === deviceId);
        if (!device) return;

        try {
            this.showLoading();
            const response = await fetch(`/api/scrape/${deviceId}`);
            if (response.ok) {
                const data = await response.json();
                device.data = data;
                device.itemCount = data.length;
                device.lastUpdate = new Date().toLocaleString();
                device.status = 'online';
                
                // 更新整个设备表格
                this.renderDevices();
                
                // 如果详情是展开的，保持展开状态
                const detailsRow = document.getElementById(`details-${deviceId}`);
                if (detailsRow && detailsRow.classList.contains('show')) {
                    this.toggleDeviceDetails(deviceId);
                }
                
                this.showNotification(`${device.name} 数据刷新成功`, 'success');
                this.addLogToInfoPanel(`${device.name} 获取到 ${data.length} 个组件`, 'success');
            } else {
                throw new Error('刷新失败');
            }
        } catch (error) {
            console.error('刷新设备失败:', error);
            this.showNotification(`刷新 ${device.name} 失败`, 'error');
            this.addLogToInfoPanel(`刷新 ${device.name} 失败: ${error.message}`, 'error');
        } finally {
            this.hideLoading();
        }
    }

    // ...existing code...
}

// 初始化应用
const app = new ICPlatform();

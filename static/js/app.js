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
        await this.loadConfigs();
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

    hideModals() {
        document.querySelectorAll('.modal').forEach(modal => {
            modal.classList.remove('show');
        });
    }

    fillForm(form, data) {
        Object.keys(data).forEach(key => {
            let input = form.querySelector(`[name="${key}"]`);
        	if (!input && key.includes('_')) {
            	input = form.querySelector(`[name="${key.replace(/_/g, '-')}"`);
        	}
            if (input) {
                input.value = data[key];
            }
        });
    }

    async loadConfigs() {
        try {
            const response = await fetch('/api/devices');
            this.configs = await response.json();
            this.renderConfigs();
            
            if (this.configs.length > 0) {
                this.loadDevices();
            }
        } catch (error) {
            this.showNotification('加载配置失败', 'error');
        }
    }

    renderConfigs() {
        const container = document.getElementById('config-list');
        
        if (this.configs.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: #7f8c8d;">暂无配置</p>';
            return;
        }

        container.innerHTML = this.configs.map(config => `
            <div class="config-item" data-config-id="${config.id}">
                <h4>${config.name}</h4>
                <div class="config-actions">
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

    async saveConfig() {
        const form = document.getElementById('config-form');
        const formData = new FormData(form);
        const data = Object.fromEntries(formData);
        
        // 验证URL格式
        const urlFields = ['login_url'];
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
        this.devices = [];

        for (const config of this.configs) {
            this.devices.push({
                id: config.id,
                name: config.name,
				type: config.dev_type,
                status: 'online',
                loginUrl: config.login_url,
                itemCount: config.vm ? config.vm.length : 0,
                lastUpdate: config.time_stamp,
				data: config.vm || [],
            });
        }
        this.renderDevices();    
    }

    getDeviceTypeLabel(type) {
		switch(type) {
			case 'csmp': return 'CSMP';
			case 'CSMP': return 'CSMP';
			case 'xc': return '信创';
			case 'XC': return '信创';
			case '': return '未知';
			default: return type;
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
				    <span class="device-status-badge ${device.type}">
                        ${this.getDeviceTypeLabel(device.type)}
                    </span>
				</td>
                <td>
                    <span class="device-status-badge ${device.status}">
                        ${device.status === 'online' ? '在线' : 
                          device.status === 'offline' ? '离线' : '离线'}
                    </span>
                </td>
                <td>${device.itemCount}</td>
                <td>${device.lastUpdate}</td>
                <td>
                    <div class="device-actions">
                        <button class="btn btn-success" onclick="app.refreshDevice(${device.id})">
                            <i class="fas fa-sync-alt"></i> 刷新
                        </button>
                    </div>
                </td>
                <td>
                    <button class="btn btn-outline webshell-btn" onclick="app.openWebShell(${device.id})" title="打开WebShell">
                        <i class="fas fa-terminal"></i> WebShell
                    </button>
                </td>
                <td>
                    <button class="btn btn-info" onclick="app.jumpToLogin(${device.id})" title="跳转到原始登录页面">
                        <i class="fas fa-external-link-alt"></i> 跳转
                    </button>
                </td>
            </tr>
            <tr class="device-details-row" id="details-${device.id}">
                <td colspan="7">
                    <div class="device-details-content">
                        <h4><i class="fas fa-list"></i> 组件信息</h4>
                        <div id="details-content-${device.id}">
                            ${device.status === 'online' && device.data ? 
                                this.renderDeviceDetails(device.data, device.id) : 
                                '<p style="color: #7f8c8d; text-align: center; padding: 1rem;">点击刷新更新组件信息</p>'}
                        </div>
                    </div>
                </td>
            </tr>
        `).join('');
    }

    renderDeviceDetails(data, deviceId) {
        if (!data || data.length === 0) {
            return '<p style="color: #7f8c8d; text-align: center; padding: 1rem;">暂无组件信息</p>';
        }

        return `
            <table class="device-details-table">
                <thead>
                    <tr>
                        <th>组件名称</th>
                        <th>状态</th>
                        <th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    ${data.map(item => `
                        <tr>
                            <td><strong>${item.name}</strong></td>
                            <td>
                                ${item.status === 'running' ? '运行中' : '关闭'}
                            </td>
                            <td>
                                ${item.status === 'running'? 
                                    `<button class="btn btn-primary" onclick="app.openVNC('${item.name}',${deviceId})">
                                        <i class="fas fa-external-link-alt"></i> VNC
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
        const webshellUrl = `/api/webshell?${params.toString()}`;
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
        } else {
            this.showNotification('无法打开WebShell窗口,请检查浏览器弹窗设置', 'error');
        }
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
        // 只保留协议和主域名，不带任何路径
        try {
            const urlObj = new URL(loginUrl);
            loginUrl = `${urlObj.protocol}//${urlObj.host}`;
        } catch (e) {
            // 如果URL解析失败，忽略处理
        }
        // 在新标签页打开登录页面
        window.open(loginUrl, '_blank');
        
        this.showNotification(`已在新标签页打开 ${config.name} 的登录页面`, 'info');
    }
    
    async openVNC(itemName, deviceId) {
		console.log(`打开WebShell`,this.configs);
        console.log(`打开VNC: ${itemName}, 设备ID: ${deviceId}`);
		const device = this.devices.find(d => d.id === deviceId);
		let address = '';
		let pass = '';
        if (!device) {
            this.showNotification('设备不存在', 'error');
            return;
        }

        const config = this.configs.find(c => c.id === deviceId);
        if (!config) {
            this.showNotification('未找到设备配置', 'error');
            return;
        }

        try {
            this.showLoading();
            const response = await fetch(`/api/vnc/${deviceId}?itemName=${encodeURIComponent(itemName)}`);
            if (response.ok) {
                const data = await response.json();          
                // 更新整个设备表格
                this.renderDevices();
                
                // 如果详情是展开的，保持展开状态
                const detailsRow = document.getElementById(`details-${deviceId}`);
                if (detailsRow && detailsRow.classList.contains('show')) {
                    this.toggleDeviceDetails(deviceId);
                }
				address = data.address;
				pass = data.pass;
            } else {
                const data = await response.json();
                this.showNotification(data.error || 'VNC打开跳转失败', 'error');
            }
        } catch (error) {
            console.error('VNC:', error);
            this.showNotification(`VNC打开失败`, 'error');
			return
        } finally {
            this.hideLoading();
        }

        // 在新窗口中打开WebShell
		const VNCWebshellUrl = `/api/vnc?address=${encodeURIComponent(address)}&pass=${encodeURIComponent(pass)}`;
        const windowFeatures = 'width=1050,height=860,scrollbars=yes,resizable=yes,menubar=no,toolbar=no,location=no,status=no';
        
        const newWindow = window.open(VNCWebshellUrl, `webshell-${itemName}`, windowFeatures);
        
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
        } else {
            this.showNotification('无法打开VNC窗口', 'error');
        }
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
            const response = await fetch(`/api/csmp/${deviceId}`);
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
            } else {
                throw new Error('刷新失败');
                const data = await response.json();
                this.showNotification(`刷新 ${device.name} 失败: ${data.error}`, 'error');
            }
        } catch (error) {
            console.error('刷新设备失败:', error);
            this.showNotification(`刷新 ${device.name} 失败`, 'error');
        } finally {
            this.hideLoading();
        }
    }

    // ...existing code...
}

// 初始化应用
const app = new ICPlatform();
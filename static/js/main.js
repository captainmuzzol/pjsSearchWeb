document.addEventListener('DOMContentLoaded', function() {
    // 防止模态框影响其他元素
    document.body.addEventListener('hidden.bs.modal', function() {
        // 强制刷新select下拉元素
        setTimeout(() => {
            const docTypeSelect = document.getElementById('docType');
            if (docTypeSelect) {
                const currentValue = docTypeSelect.value;
                docTypeSelect.value = '';
                docTypeSelect.value = currentValue;
            }
        }, 100);
    });
    // DOM Elements
    const mainSearchInput = document.getElementById('mainSearch');
    const excludeSearchInput = document.getElementById('excludeSearch');
    const searchTypeSelect = document.getElementById('searchType');
    const docTypeSelect = document.getElementById('docType');
    const sourceSelect = document.getElementById('source');
    const searchBtn = document.getElementById('searchBtn');
    const searchResults = document.getElementById('searchResults');
    const resultCount = document.getElementById('resultCount');
    const uploadForm = document.getElementById('uploadForm');
    const fileInput = document.getElementById('fileInput');
    const uploadStatus = document.getElementById('uploadStatus');
    
    // Document Modal Elements
    const documentModalEl = document.getElementById('documentModal');
    const documentModal = new bootstrap.Modal(documentModalEl);
    
    // 当模态框隐藏时触发事件
    documentModalEl.addEventListener('hidden.bs.modal', function() {
        // 确保关闭模态框后不影响页面其他元素
        const docTypeSelect = document.getElementById('docType');
        if (docTypeSelect && docTypeSelect.options.length === 0) {
            // 重新创建选项
            const options = [
                { value: '全部', text: '全部' },
                { value: '刑事', text: '刑事' },
                { value: '民事', text: '民事' }
            ];
            
            options.forEach(opt => {
                const option = document.createElement('option');
                option.value = opt.value;
                option.textContent = opt.text;
                docTypeSelect.appendChild(option);
            });
        }
    });
    const docTitle = document.getElementById('docTitle');
    const docSource = document.getElementById('docSource');
    const docType = document.getElementById('docType');
    const docContent = document.getElementById('docContent');
    
    // Event Listeners
    searchBtn.addEventListener('click', performSearch);
    mainSearchInput.addEventListener('keypress', function(e) {
        if (e.key === 'Enter') {
            performSearch();
        }
    });
    
    // 导入判决书相关元素
    const uploadBtn = document.getElementById('uploadBtn');
    const clearDbBtn = document.getElementById('clearDbBtn');
    const folderInput = document.getElementById('folderInput');
    const fileCount = document.getElementById('fileCount');
    const validFileCount = document.getElementById('validFileCount');
    const progressBar = document.querySelector('.progress-bar');
    const progressContainer = document.querySelector('.progress');
    
    // 文件选择事件
    folderInput.addEventListener('change', updateFileInfo);
    
    // 上传按钮事件
    uploadBtn.addEventListener('click', uploadDocuments);
    
    // 清空数据库按钮事件
    clearDbBtn.addEventListener('click', clearUserDatabase);
    
    // Search Function
    function performSearch() {
        const query = mainSearchInput.value.trim();
        const excludeQuery = excludeSearchInput.value.trim();
        const searchType = searchTypeSelect.value;
        const docType = docTypeSelect.value;
        const source = sourceSelect.value;
        
        if (query === '' && excludeQuery === '') {
            alert('请输入搜索关键词');
            return;
        }
        
        // Show loading state
        searchResults.innerHTML = '<div class="text-center my-5"><div class="spinner-border" role="status"></div><p class="mt-2">正在搜索...</p></div>';
        
        // Build query URL
        const url = `/api/search?q=${encodeURIComponent(query)}&exclude=${encodeURIComponent(excludeQuery)}&type=${searchType}&docType=${encodeURIComponent(docType)}&source=${encodeURIComponent(source)}`;
        
        // Fetch results
        fetch(url)
            .then(response => {
                if (!response.ok) {
                    throw new Error(`HTTP error! Status: ${response.status}`);
                }
                return response.json();
            })
            .then(data => {
                // 增加对null和空值的检查
                if (data === null || data === undefined) {
                    // 如果数据是null或undefined，当作没有结果处理
                    console.log('服务器返回空数据，将其当作没有结果处理');
                    displayResults([], query);
                    return;
                }
                
                // 各种情况的处理
                if (Array.isArray(data)) {
                    // 正常情况：服务器返回了数组（可能为空）
                    displayResults(data, query);
                } else if (data && data.error) {
                    // 服务器返回了错误消息
                    console.error('服务器错误:', data.error);
                    searchResults.innerHTML = '<div class="alert alert-danger">搜索时发生错误，请重新刷新页面后再试</div>';
                } else {
                    // 如果是其他意外格式，当作没有结果处理
                    console.log('服务器返回意外格式数据，当作没有结果处理');
                    displayResults([], query);
                }
            })
            .catch(error => {
                // 捕获并记录错误
                console.error('搜索错误:', error);
                
                // 将错误转化为“没有结果”的友好提示
                // 用户不需要知道技术错误，只需要知道没有找到结果
                displayResults([], query);
            });
    }
    
    // Display Search Results
    function displayResults(results, query) {
        searchResults.innerHTML = '';
        resultCount.textContent = results.length;
        
        if (results.length === 0) {
            searchResults.innerHTML = '<div class="alert alert-info">没有找到任何文件哦，请重新核对搜索条件后再试吧！</div>';
            return;
        }
        
        // Create a document fragment for better performance
        const fragment = document.createDocumentFragment();
        
        // Process each result
        results.forEach(doc => {
            const item = document.createElement('div');
            item.className = 'list-group-item document-item';
            
            // Create snippet from content
            let snippet = doc.content.substring(0, 200) + '...';
            
            // Highlight search terms in snippet if query exists
            if (query) {
                const keywords = query.split(' ').filter(k => k.trim() !== '');
                keywords.forEach(keyword => {
                    const regex = new RegExp(keyword, 'gi');
                    snippet = snippet.replace(regex, match => `<span class="highlight">${match}</span>`);
                });
            }
            
            // Create HTML for the item
            item.innerHTML = `
                <div class="document-title">${doc.title}</div>
                <div class="document-snippet">${snippet}</div>
                <div class="document-meta">
                    <span class="badge bg-info me-2">${doc.source}</span>
                    <span class="badge bg-secondary">${doc.type}</span>
                </div>
            `;
            
            // Add click event to view document
            item.addEventListener('click', () => {
                // 使用当前搜索时的关键词，而不是输入框中的值
                viewDocument(doc.id, doc.source, query);
            });
            
            fragment.appendChild(item);
        });
        
        searchResults.appendChild(fragment);
    }
    
    // View Document Function
    function viewDocument(id, source, query) {
        // Store the search query in a variable for debugging
        console.log(`Opening document ${id} from ${source} with search query: "${query}"`);
        
        // Fetch document details
        fetch(`/api/document/${id}?source=${encodeURIComponent(source)}&q=${encodeURIComponent(query || '')}`)
            .then(response => {
                if (!response.ok) {
                    throw new Error(`HTTP error! Status: ${response.status}`);
                }
                return response.json();
            })
            .then(doc => {
                // Update modal content
                docTitle.textContent = doc.title;
                docSource.textContent = doc.source;
                docType.textContent = doc.type;
                
                // 处理内容格式
                let content = doc.content || '';
                if (typeof content !== 'string') {
                    content = String(content);
                }
                
                // 应用基本格式化
                content = content.replace(/\n/g, '<br>');
                
                // 高亮搜索关键词
                if (query && query.trim() !== '') {
                    const keywords = query.split(' ').filter(k => k.trim() !== '');
                    
                    keywords.forEach(keyword => {
                        if (keyword && keyword.trim()) {
                            try {
                                // 转义特殊字符
                                const escapedKeyword = keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
                                const regex = new RegExp(escapedKeyword, 'gi');
                                content = content.replace(regex, match => `<span class="highlight">${match}</span>`);
                            } catch (e) {
                                console.error(`高亮关键词出错:`, e);
                            }
                        }
                    });
                }
                
                // 设置处理后的内容
                docContent.innerHTML = content;
                
                // 显示模态框
                documentModal.show();
            })
            .catch(error => {
                console.error('Error fetching document details:', error);
                alert('获取文档详情失败，请重试');
            });
    }
    
    // 更新文件信息显示
    function updateFileInfo() {
        const files = folderInput.files;
        const totalFiles = files.length;
        let validFiles = 0;
        
        // 更新文件计数
        fileCount.textContent = totalFiles;
        
        // 计算有效文件数量
        for (let i = 0; i < totalFiles; i++) {
            const fileName = files[i].name.toLowerCase();
            if (fileName.endsWith('.doc') || fileName.endsWith('.docx')) {
                validFiles++;
            }
        }
        
        validFileCount.textContent = validFiles;
    }
    
    // 清空自定义数据库函数
    function clearUserDatabase() {
        // 先弹出确认对话框
        if (!confirm('警告：此操作将清空所有已导入的数据，并且不可恢复。确定要继续吗？')) {
            return; // 用户取消
        }
        
        // 显示加载状态
        uploadStatus.innerHTML = '<div class="text-center"><div class="spinner-border spinner-border-sm" role="status"></div> <span>正在清空数据库...</span></div>';
        
        // 发送请求到后端
        fetch('/api/clear-db', {
            method: 'POST'
        })
        .then(response => response.json())
        .then(data => {
            if (data.error) {
                uploadStatus.innerHTML = `<div class="alert alert-danger">清空数据库失败: ${data.error}</div>`;
            } else {
                uploadStatus.innerHTML = '<div class="alert alert-success">数据库已成功清空</div>';
                
                // 2秒后刷新页面
                setTimeout(() => {
                    location.reload();
                }, 2000);
            }
        })
        .catch(error => {
            console.error('清空数据库错误:', error);
            uploadStatus.innerHTML = '<div class="alert alert-danger">清空数据库时发生错误</div>';
        });
    }
    
    // 批量上传文档函数
    function uploadDocuments() {
        const files = folderInput.files;
        const totalFiles = files.length;
        
        if (totalFiles === 0) {
            uploadStatus.innerHTML = '<div class="alert alert-danger">请选择文件夹</div>';
            return;
        }
        
        // 筛选有效的.doc和.docx文件
        const validFiles = Array.from(files).filter(file => {
            const fileName = file.name.toLowerCase();
            return fileName.endsWith('.doc') || fileName.endsWith('.docx');
        });
        
        if (validFiles.length === 0) {
            uploadStatus.innerHTML = '<div class="alert alert-warning">选中皮文件夹中没有.doc或.docx文件</div>';
            return;
        }
        
        // 显示进度条
        progressContainer.style.display = 'block';
        progressBar.style.width = '0%';
        progressBar.textContent = '0%';
        progressBar.setAttribute('aria-valuenow', 0);
        
        // 显示开始上传状态
        uploadStatus.innerHTML = `<div class="alert alert-info">正在准备上传 ${validFiles.length} 个文件...</div>`;
        
        // 开始上传文件
        uploadFilesSequentially(validFiles);
    }
    
    // 顺序上传多个文件
    function uploadFilesSequentially(files) {
        let currentIndex = 0;
        let successCount = 0;
        let failCount = 0;
        let uploadLogs = [];
        
        function uploadNext() {
            if (currentIndex >= files.length) {
                // 所有文件处理结束后，设置进度条为100%
                progressBar.style.width = '100%';
                progressBar.textContent = '100%';
                progressBar.setAttribute('aria-valuenow', 100);
                
                // 全部上传完成
                const resultMessage = `上传完成: 成功 ${successCount} 个, 失败 ${failCount} 个`;
                uploadStatus.innerHTML = `<div class="alert alert-${failCount > 0 ? 'warning' : 'success'}">${resultMessage}</div>`;
                
                // 显示日志
                if (uploadLogs.length > 0) {
                    const logDiv = document.createElement('div');
                    logDiv.className = 'mt-2 small';
                    logDiv.style.maxHeight = '150px';
                    logDiv.style.overflowY = 'auto';
                    logDiv.innerHTML = '<strong>详细日志:</strong><br>' + uploadLogs.join('<br>');
                    uploadStatus.appendChild(logDiv);
                }
                
                // 3秒后关闭模态框
                setTimeout(() => {
                    const uploadModal = bootstrap.Modal.getInstance(document.getElementById('uploadModal'));
                    if (uploadModal) {
                        uploadModal.hide();
                    }
                    
                    // 刷新数据来源列表，选中"已导入数据"
                    const sourceSelect = document.getElementById('source');
                    sourceSelect.value = '已导入数据';
                }, 3000);
                
                return;
            }
            
            // 更新进度 - 修复计算方式确保能到达100%
            const progress = Math.round((currentIndex / files.length) * 100);
            progressBar.style.width = `${progress}%`;
            progressBar.textContent = `${progress}%`;
            progressBar.setAttribute('aria-valuenow', progress);
            
            // 上传当前文件
            const file = files[currentIndex];
            const formData = new FormData();
            formData.append('file', file);
            
            // 显示当前文件名
            uploadStatus.innerHTML = `<div class="alert alert-info">正在上传 (${currentIndex+1}/${files.length}): ${file.name}</div>`;
            
            fetch('/api/upload', {
                method: 'POST',
                body: formData
            })
            .then(response => response.json())
            .then(data => {
                if (data.error) {
                    failCount++;
                    uploadLogs.push(`<span class="text-danger">✖ ${file.name} - 失败: ${data.error}</span>`);
                } else {
                    successCount++;
                    uploadLogs.push(`<span class="text-success">✔ ${file.name} - 成功</span>`);
                }
                
                // 上传下一个文件
                currentIndex++;
                uploadNext();
            })
            .catch(error => {
                console.error('Error:', error);
                failCount++;
                uploadLogs.push(`<span class="text-danger">✖ ${file.name} - 失败: 网络错误</span>`);
                
                // 继续上传下一个文件
                currentIndex++;
                uploadNext();
            });
        }
        
        // 开始上传第一个文件
        uploadNext();
    }
});

let eventSource = null;

function escapeHtml(s) {
  if (typeof s !== 'string') return '';
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

// Theme
var currentTheme = localStorage.getItem('imgp-theme') || 'light';
if (currentTheme === 'dark') {
  document.documentElement.setAttribute('data-theme', 'dark');
  document.querySelector('.theme-toggle').textContent = '☀️';
}

function toggleTheme() {
  var btn = document.querySelector('.theme-toggle');
  if (currentTheme === 'light') {
    document.documentElement.setAttribute('data-theme', 'dark');
    btn.textContent = '☀️';
    currentTheme = 'dark';
  } else {
    document.documentElement.removeAttribute('data-theme');
    btn.textContent = '🌙';
    currentTheme = 'light';
  }
  localStorage.setItem('imgp-theme', currentTheme);
}

function startDownload() {
  const image = document.getElementById('imageName').value.trim();
  if (!image) { alert('请输入镜像名'); return; }

  const platform = document.getElementById('platform').value;
  const output = document.getElementById('outputFile').value.trim();
  const username = document.getElementById('username').value.trim();
  const password = document.getElementById('password').value;
  const insecure = document.getElementById('insecure').checked;

  document.querySelector('.btn-primary').disabled = true;
  document.querySelector('.btn-primary').textContent = '⏳ 下载中...';

  const progressSection = document.getElementById('progressSection');
  progressSection.style.display = 'block';
  document.getElementById('errorBox').style.display = 'none';
  document.getElementById('doneBox').style.display = 'none';
  var cancelBtn = document.getElementById('cancelBtn');
  cancelBtn.disabled = false;
  cancelBtn.textContent = '✕ 取消';
  cancelBtn.style.display = 'inline';

  fetch('/api/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ image, platform, output, username, password, insecure })
  })
  .then(r => r.json())
  .then(data => {
    if (data.ok) connectSSE();
    else showError(data.error || '启动失败');
  })
  .catch(err => showError(err.message));
}

function connectSSE() {
  if (eventSource) eventSource.close();
  eventSource = new EventSource('/api/progress');

  eventSource.onmessage = function(e) {
    try {
      const data = JSON.parse(e.data);
      updateProgress(data);
    } catch(err) {}
  };

  eventSource.onerror = function() {
    var doneBox = document.getElementById('doneBox');
    var errorBox = document.getElementById('errorBox');
    if (doneBox.style.display !== 'none' || errorBox.style.display !== 'none') {
      if (eventSource) { eventSource.close(); eventSource = null; }
    }
  };
}

function updateProgress(data) {
  const header = document.getElementById('progressHeader');
  const layerList = document.getElementById('layerList');
  const exportDiv = document.getElementById('exportProgress');
  const errorBox = document.getElementById('errorBox');
  const doneBox = document.getElementById('doneBox');

  if (data.phase === 'starting') {
    header.textContent = '⏳ 准备中...';
    return;
  }

  if (data.phase === 'error') {
    exportDiv.style.display = 'none';
    showError(data.error || '下载失败');
    return;
  }

  if (data.phase === 'done') {
    document.querySelector('.btn-primary').disabled = false;
    document.querySelector('.btn-primary').textContent = '▶ 开始下载';
    document.getElementById('cancelBtn').style.display = 'none';
    if (eventSource) eventSource.close();
    exportDiv.style.display = 'none';
    layerList.innerHTML = '';
    header.innerHTML = '';
    doneBox.style.display = 'block';
    var outPath = escapeHtml(data.outputPath);
    doneBox.innerHTML = '✅ 下载完成！已保存到 ' + outPath +
      ' <button class="btn btn-success" onclick="openFileLocation(\'' + escapeHtml(data.outputPath) + '\')" style="margin-left:8px;padding:4px 12px;font-size:12px">📂 打开位置</button>';
    return;
  }

  if (data.phase === 'downloading') {
    const percent = data.totalBytes > 0 ? (data.doneBytes / data.totalBytes * 100).toFixed(1) : 0;
    header.innerHTML = '<b>正在下载:</b> ' + data.doneLayers + '/' + data.totalLayers + ' 层 | ' +
      (data.totalBytes > 0 ? percent + '% - ' + fmtSize(data.doneBytes) + ' / ' + fmtSize(data.totalBytes) : '准备中...');

    var html = '';
    data.layers.forEach(function(l) {
      const pct = l.total > 0 ? (l.bytes / l.total * 100) : 0;
      const cls = l.status === 'done' ? 'done' : l.status === 'cached' ? 'cached' : l.status === 'downloading' ? 'downloading' : '';
      html += '<div class="progress-bar">';
      html += '<span style="width:60px;font-family:monospace;font-size:11px;color:var(--text-secondary)">' + shortenDigest(l.digest) + '</span>';
      html += '<div class="progress-track"><div class="progress-fill ' + cls + '" style="width:' + pct + '%"></div></div>';
      html += '<span class="progress-label">' + (l.status === 'waiting' ? '等待中' : l.total === 0 ? '' : pct.toFixed(0) + '%') + '</span>';
      if (l.status === 'downloading' && l.total > 0) {
        html += '<span style="font-size:11px;color:var(--text-secondary);min-width:70px;text-align:right">' + fmtSize(l.bytes) + '/' + fmtSize(l.total) + '</span>';
      }
      html += '</div>';
    });
    layerList.innerHTML = html;
    exportDiv.style.display = 'none';
  }

  if (data.phase === 'exporting') {
    header.innerHTML = '<b>正在导出 tar...</b>';
    exportDiv.style.display = 'block';
    if (data.exportTotal > 0) {
      const pct = (data.exportBytes / data.exportTotal * 100).toFixed(0);
      exportDiv.innerHTML = '<div class="progress-bar"><span>导出中</span><div class="progress-track"><div class="progress-fill downloading" style="width:' + pct + '%"></div></div><span class="progress-label">' + pct + '%</span></div>';
    } else {
      exportDiv.innerHTML = '<div class="progress-bar"><span>导出中</span><span style="color:var(--text-secondary);font-size:13px">准备中...</span></div>';
    }
  }
}

function showError(msg) {
  document.querySelector('.btn-primary').disabled = false;
  document.querySelector('.btn-primary').textContent = '▶ 开始下载';
  document.getElementById('cancelBtn').style.display = 'none';
  if (eventSource) eventSource.close();
  document.getElementById('errorBox').style.display = 'block';
  document.getElementById('errorBox').textContent = '❌ ' + msg;
}

function openFileLocation(path) {
  fetch('/api/open-file', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: path })
  });
}

function formatBytes(b) {
  if (b < 1024) return b + ' B';
  if (b < 1024*1024) return (b/1024).toFixed(1) + ' KB';
  if (b < 1024*1024*1024) return (b/1024/1024).toFixed(1) + ' MB';
  return (b/1024/1024/1024).toFixed(1) + ' GB';
}

function shortenDigest(s) {
  if (!s || s.length <= 12) return s || '';
  return s.substring(0, 12);
}

function fmtSize(n) { return formatBytes(n); }

function shutdownServer() {
  if (!confirm('确定关闭服务？')) return;
  fetch('/api/shutdown', { method: 'POST' });
}

function loadCacheInfo() {
  fetch('/api/cache')
    .then(r => r.json())
    .then(data => {
      document.getElementById('cacheInfo').textContent = '缓存目录: ' + data.path + ' | 文件数: ' + data.files + ' | 大小: ' + formatBytes(data.size);
    })
    .catch(() => {
      document.getElementById('cacheInfo').textContent = '无法读取缓存信息';
    });
}

function clearCache() {
  if (!confirm('确定清空所有缓存？')) return;
  fetch('/api/cache', { method: 'POST' })
    .then(r => { if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'unknown'); }); return r.json(); })
    .then(() => { loadCacheInfo(); })
    .catch(err => alert('清空失败: ' + err.message));
}

function showConfig() {
  document.getElementById('configModal').style.display = 'flex';
  document.getElementById('configStatus').textContent = '';

  fetch('/api/config')
    .then(r => r.json())
    .then(data => {
      const tbody = document.getElementById('mirrorTableBody');
      tbody.innerHTML = '';
      if (data.mirror_map) {
        Object.keys(data.mirror_map).forEach(function(reg) {
          const mirrors = data.mirror_map[reg];
          const val = Array.isArray(mirrors) ? mirrors.join('|') : mirrors;
          addMirrorRow(reg, val);
        });
      }
      document.getElementById('cfgParallelism').value = data.parallelism || 4;
      document.getElementById('cfgInsecureRegistries').value = (data.insecure_registries || []).join(', ');
      document.getElementById('cfgLayerTimeout').value = data.layer_timeout || 30;
      document.getElementById('cfgTimeout').value = data.timeout || 0;
      document.getElementById('cfgRetry').value = data.retry !== undefined ? data.retry : 2;
    });
}

function hideConfig() {
  document.getElementById('configModal').style.display = 'none';
}

function addMirrorRow(reg, mirror) {
  const tbody = document.getElementById('mirrorTableBody');
  const tr = document.createElement('tr');
  tr.innerHTML = '<td><input type="text" class="cfg-reg" value="' + escapeHtml(reg || '') + '" placeholder="docker.io"></td>' +
    '<td><input type="text" class="cfg-mirror" value="' + escapeHtml(mirror || '') + '" placeholder="mirror.example.com"></td>' +
    '<td><button onclick="this.parentElement.parentElement.remove()" style="color:var(--danger);border:none;background:none;cursor:pointer;font-size:16px">×</button></td>';
  tbody.appendChild(tr);
}

function saveConfig() {
  const rows = document.querySelectorAll('#mirrorTableBody tr');
  const mirrorMap = {};
  rows.forEach(function(tr) {
    const reg = tr.querySelector('.cfg-reg').value.trim();
    const mirror = tr.querySelector('.cfg-mirror').value.trim();
    if (reg && mirror) mirrorMap[reg] = mirror;
  });

  const parallelism = parseInt(document.getElementById('cfgParallelism').value) || 4;
  const insecureRaw = document.getElementById('cfgInsecureRegistries').value.trim();
  const insecureRegistries = insecureRaw ? insecureRaw.split(',').map(s => s.trim()).filter(s => s) : [];
  const layerTimeout = parseInt(document.getElementById('cfgLayerTimeout').value) || 30;
  const timeout = parseInt(document.getElementById('cfgTimeout').value) || 0;
  const retry = parseInt(document.getElementById('cfgRetry').value);
  const retryVal = isNaN(retry) ? 2 : retry;

  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      mirror_map: mirrorMap,
      parallelism: parallelism,
      insecure_registries: insecureRegistries,
      layer_timeout: layerTimeout,
      timeout: timeout,
      retry: retryVal
    })
  })
  .then(r => r.json())
  .then(data => {
    document.getElementById('configStatus').textContent = data.ok ? '✅ 已保存' : '❌ 保存失败';
    if (data.ok) setTimeout(hideConfig, 1500);
  })
  .catch(() => { document.getElementById('configStatus').textContent = '❌ 保存失败'; });
}

function cancelDownload() {
  var btn = document.getElementById('cancelBtn');
  btn.disabled = true;
  btn.textContent = '⏳ 取消中...';
  fetch('/api/cancel', { method: 'POST' }).then(function() {
    if (eventSource) eventSource.close();
    btn.style.display = 'none';
    document.getElementById('errorBox').style.display = 'block';
    document.getElementById('errorBox').textContent = '❌ 用户取消';
    document.querySelector('.btn-primary').disabled = false;
    document.querySelector('.btn-primary').textContent = '▶ 开始下载';
  }).catch(function() {
    if (eventSource) eventSource.close();
    btn.disabled = false;
    btn.textContent = '✖ 取消下载';
  });
}

loadCacheInfo();

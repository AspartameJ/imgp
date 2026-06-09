let eventSource = null;

window.addEventListener('beforeunload', function() {
  navigator.sendBeacon('/api/shutdown');
});

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
  document.getElementById('cancelBtn').style.display = 'inline';

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
    eventSource.close();
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
    showError(data.error || '下载失败');
    return;
  }

  if (data.phase === 'done') {
    document.querySelector('.btn-primary').disabled = false;
    document.querySelector('.btn-primary').textContent = '▶ 开始下载';
    document.getElementById('cancelBtn').style.display = 'none';
    if (eventSource) eventSource.close();
    doneBox.style.display = 'block';
    doneBox.textContent = '✅ 下载完成！已保存到 ' + data.outputPath;
    loadCacheInfo();
    return;
  }

  if (data.phase === 'downloading') {
    const percent = data.totalBytes > 0 ? (data.doneBytes / data.totalBytes * 100).toFixed(1) : 0;
    header.innerHTML = `<b>正在下载:</b> ${data.doneLayers}/${data.totalLayers} 层 | ${data.totalBytes > 0 ? percent + '% - ' + fmtSize(data.doneBytes) + ' / ' + fmtSize(data.totalBytes) : '准备中...'}`;

    let html = '';
    data.layers.forEach(function(l) {
      const pct = l.total > 0 ? (l.bytes / l.total * 100) : 0;
      const cls = l.status === 'done' ? 'done' : l.status === 'cached' ? 'cached' : l.status === 'downloading' ? 'downloading' : '';
      html += '<div class="progress-bar">';
      html += '<span style="width:60px">' + shortenDigest(l.digest) + '</span>';
      html += '<div class="progress-track"><div class="progress-fill ' + cls + '" style="width:' + pct + '%"></div></div>';
      html += '<span class="progress-label">' + (l.status === 'waiting' ? '等待中' : l.total === 0 ? '' : pct.toFixed(0) + '%') + '</span>';
      if (l.status === 'downloading' && l.total > 0) {
        html += '<span style="font-size:11px;color:#888;min-width:70px;text-align:right">' + fmtSize(l.bytes) + '/' + fmtSize(l.total) + '</span>';
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
      exportDiv.innerHTML = '<div class="progress-bar"><span>导出中</span><span style="color:#888;font-size:13px">准备中...</span></div>';
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
    .then(r => r.json())
    .then(() => { loadCacheInfo(); })
    .catch(() => alert('清空失败'));
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
    });
}

function hideConfig() {
  document.getElementById('configModal').style.display = 'none';
}

function addMirrorRow(reg, mirror) {
  const tbody = document.getElementById('mirrorTableBody');
  const tr = document.createElement('tr');
  tr.innerHTML = '<td><input type="text" class="cfg-reg" value="' + (reg || '') + '" placeholder="docker.io"></td>' +
    '<td><input type="text" class="cfg-mirror" value="' + (mirror || '') + '" placeholder="mirror.example.com"></td>' +
    '<td><button onclick="this.parentElement.parentElement.remove()" style="color:#e74c3c;border:none;background:none;cursor:pointer">✕</button></td>';
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

  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mirror_map: mirrorMap, parallelism: parallelism })
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
  if (eventSource) eventSource.close();
  fetch('/api/cancel', { method: 'POST' }).then(function() {
    btn.style.display = 'none';
    document.getElementById('errorBox').style.display = 'block';
    document.getElementById('errorBox').textContent = '❌ 用户取消';
    document.querySelector('.btn-primary').disabled = false;
    document.querySelector('.btn-primary').textContent = '▶ 开始下载';
  }).catch(function() {
    btn.disabled = false;
    btn.textContent = '✕ 取消';
  });
}

loadCacheInfo();

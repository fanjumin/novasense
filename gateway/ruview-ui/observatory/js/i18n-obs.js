// i18n-obs.js — observatory.html 的轻量本地化（英文原文 -> 目标语言）。
// 不改动场景/渲染代码：按“整段 trim 后精确匹配英文”翻译文本节点与 title/placeholder/aria-label，
// 用 MutationObserver 覆盖 HUD 等动态文案；可切回英文（存原文）。语言由共享 i18n 决定/持久化。
import { i18n } from '../../utils/i18n.js';

const ZH = {
  // 标题/面板
  'WiFi DensePose Sensing Observatory': 'WiFi-DensePose 感知观测台',
  'Data Source': '数据源', 'Live WebSocket': '实时 WebSocket', 'Demo Generator': '演示生成器',
  'Scenario': '场景', 'Scene': '场景', 'Settings': '设置', 'Rendering': '渲染',
  'Style Preset': '风格预设', 'Reset Camera': '重置视角', 'Reset to Defaults': '恢复默认',
  'Export Settings': '导出设置', 'Show Grid': '显示网格', 'Show Room Boundary': '显示房间边界',
  'Auto-Cycle': '自动轮播', 'Auto-Cycle (30s)': '自动轮播（30秒）', 'Auto-cycling': '自动轮播',
  'Cycle Speed (s)': '轮播间隔（秒）', 'Custom': '自定义', 'Change scenario': '切换场景',
  // 场景选项
  'Empty Room': '空房间', 'Vital Signs': '生命体征', 'Multi-Person': '多人',
  'Fall Detect': '跌倒检测', 'Sleep Monitor': '睡眠监测', 'Intrusion': '入侵',
  'Gesture Ctrl': '手势控制', 'Crowd (4 ppl)': '人群（4 人）', 'Search Rescue': '搜救',
  'Elderly Care': '老人照护', 'Fitness': '健身', 'Security Patrol': '安保巡逻',
  // 场景全称
  'Presence Detection': '存在检测', 'Vital Signs (Breathing)': '生命体征（呼吸）',
  'Vital Sign Monitoring': '生命体征监测', 'Multi-Person Tracking': '多人追踪',
  'Fall Detection': '跌倒检测', 'Sleep Monitoring (Apnea)': '睡眠监测（呼吸暂停）',
  'Intrusion Detection': '入侵检测', 'Gesture Control (DTW)': '手势控制（DTW）',
  'Crowd Occupancy (4 ppl)': '人群占用（4 人）', 'Elderly Care (Gait)': '老人照护（步态）',
  'Fitness Tracking': '健身追踪', 'Human Pose Estimation': '人体姿态估计', 'Medical Monitor': '医疗监护',
  // HUD/状态
  'Presence': '在场', 'Motion': '移动', 'Confidence': '置信度', 'Persons': '人数',
  'Heart Rate': '心率', 'Respiration': '呼吸', 'RSSI': '信号强度', 'BPM': 'BPM', 'RPM': 'RPM',
  'Variance': '方差', 'WiFi Signal': 'WiFi 信号', 'WiFi Waves': 'WiFi 波', 'Signal Field': '信号场',
  'FALL DETECTED': '检测到跌倒', 'ABSENT': '无人',
  // 渲染设置
  'Aura Opacity': '光晕不透明度', 'Bloom Radius': '泛光半径', 'Bloom Strength': '泛光强度',
  'Bloom Threshold': '泛光阈值', 'Bone Thickness': '骨骼粗细', 'Chromatic Aberration': '色差',
  'Cinematic': '电影感', 'Exposure': '曝光', 'Film Grain': '胶片颗粒', 'Floor Reflection': '地面反射',
  'FOV': '视场角', 'Glow Intensity': '辉光强度', 'Joint Color': '关节颜色', 'Joint Size': '关节大小',
  'Minimal / Clean': '极简 / 干净', 'Neon Glow': '霓虹辉光', 'Orbit Speed': '环绕速度',
  'Particle Trail': '粒子拖尾', 'Room Brightness': '房间亮度', 'Tactical / Military': '战术 / 军用',
  'Vignette': '暗角', 'Wireframe': '线框', 'Wireframe Color': '线框颜色', 'Foundation (Default)': '基础（默认）',
  // 观测台 HUD 补充（大写场景名 / 场景描述 / 边缘徽标 / 状态标签）
  'EMPTY ROOM': '空房间', 'VITAL SIGNS': '生命体征', 'MULTI-PERSON': '多人',
  'FALL DETECT': '跌倒检测', 'SLEEP MONITOR': '睡眠监测', 'INTRUSION': '入侵',
  'GESTURE CTRL': '手势控制', 'CROWD OCCUPANCY': '人群占用', 'SEARCH RESCUE': '搜救',
  'ELDERLY CARE': '老人照护', 'FITNESS': '健身', 'SECURITY PATROL': '安保巡逻',
  'Auto-cycling through all sensing scenarios.': '自动轮播所有感知场景。',
  'Baseline calibration with no human presence in the monitored zone.': '监控区内无人时的基线校准。',
  'Detecting vital signs through WiFi signal micro-variations.': '通过 WiFi 信号微变检测生命体征。',
  'Tracking multiple people simultaneously via CSI multiplex separation.': '通过 CSI 复用分离同时追踪多人。',
  'Sudden posture-change detection using acceleration feature analysis.': '利用加速度特征分析检测突发姿态变化。',
  'Monitoring breathing patterns and apnea events during sleep.': '监测睡眠中的呼吸模式与呼吸暂停事件。',
  'Passive perimeter monitoring -- no cameras, pure RF sensing.': '被动式周界监测——无摄像头，纯射频感知。',
  'DTW-based gesture recognition from hand/arm motion signatures.': '基于 DTW 的手势识别，源自手臂运动特征。',
  'Estimating room occupancy count from aggregate CSI variance.': '根据聚合 CSI 方差估算房间占用人数。',
  'Through-wall survivor detection using WiFi-MAT multistatic mode.': '使用 WiFi-MAT 多基地模式进行隔墙幸存者检测。',
  'Continuous gait analysis for early mobility-decline detection.': '持续步态分析，用于早期行动能力衰退检测。',
  'Rep counting and exercise classification from body kinematics.': '基于身体运动学进行次数计数与动作分类。',
  'Multi-zone presence patrol with camera-free motion heatmaps.': '多区域在场巡逻，无摄像头的运动热力图。',
  'VITALS': '生命体征', 'GAIT': '步态', 'TRACKING': '追踪', 'FALL': '跌倒',
  'APNEA': '呼吸暂停', 'PRESENCE': '在场', 'ALERT': '警报', 'GESTURE': '手势', 'OCCUPANCY': '占用',
  'LIVE': '实时', 'DEMO': '演示', 'ACTIVE': '活跃', 'PRESENT': '在场',
};
const PL = {
  // Tytuły/paneele
  'WiFi DensePose Sensing Observatory': 'Obserwatorium wykrywania WiFi DensePose',
  'Data Source': 'Źródło danych', 'Live WebSocket': 'WebSocket na żywo', 'Demo Generator': 'Generator demo',
  'Scenario': 'Scenariusz', 'Scene': 'Scena', 'Settings': 'Ustawienia', 'Rendering': 'Renderowanie',
  'Style Preset': 'Preset stylu', 'Reset Camera': 'Resetuj kamerę', 'Reset to Defaults': 'Przywróć domyślne',
  'Export Settings': 'Ustawienia eksportu', 'Show Grid': 'Pokaż siatkę', 'Show Room Boundary': 'Pokaż granicę pokoju',
  'Auto-Cycle': 'Auto-cykl', 'Auto-Cycle (30s)': 'Auto-cykl (30 s)', 'Auto-cycling': 'Auto-cyklowanie',
  'Cycle Speed (s)': 'Szybkość cyklu (s)', 'Custom': 'Własny', 'Change scenario': 'Zmień scenariusz',
  // Opcje scenariuszy
  'Empty Room': 'Pusty pokój', 'Vital Signs': 'Parametry życiowe', 'Multi-Person': 'Wiele osób',
  'Fall Detect': 'Wykrywanie upadku', 'Sleep Monitor': 'Monitor snu', 'Intrusion': 'Włamanie',
  'Gesture Ctrl': 'Sterowanie gestami', 'Crowd (4 ppl)': 'Tłum (4 os.)', 'Search Rescue': 'Poszukiwanie i ratunek',
  'Elderly Care': 'Opieka nad seniorami', 'Fitness': 'Fitness', 'Security Patrol': 'Patrol ochrony',
  // Pełne nazwy scenariuszy
  'Presence Detection': 'Wykrywanie obecności', 'Vital Signs (Breathing)': 'Parametry życiowe (oddech)',
  'Vital Sign Monitoring': 'Monitorowanie parametrów życiowych', 'Multi-Person Tracking': 'Śledzenie wielu osób',
  'Fall Detection': 'Wykrywanie upadków', 'Sleep Monitoring (Apnea)': 'Monitorowanie snu (bezdech)',
  'Intrusion Detection': 'Wykrywanie włamań', 'Gesture Control (DTW)': 'Sterowanie gestami (DTW)',
  'Crowd Occupancy (4 ppl)': 'Zajętość tłumu (4 os.)', 'Elderly Care (Gait)': 'Opieka nad seniorami (chód)',
  'Fitness Tracking': 'Śledzenie kondycji', 'Human Pose Estimation': 'Estymacja pozycji człowieka', 'Medical Monitor': 'Monitor medyczny',
  // HUD/status
  'Presence': 'Obecność', 'Motion': 'Ruch', 'Confidence': 'Pewność', 'Persons': 'Osoby',
  'Heart Rate': 'Tętno', 'Respiration': 'Oddychanie', 'RSSI': 'RSSI', 'BPM': 'BPM', 'RPM': 'RPM',
  'Variance': 'Wariancja', 'WiFi Signal': 'Sygnał WiFi', 'WiFi Waves': 'Fale WiFi', 'Signal Field': 'Pole sygnału',
  'FALL DETECTED': 'WYKRYTO UPADEK', 'ABSENT': 'BRAK',
  // Ustawienia renderowania
  'Aura Opacity': 'Nieprzezroczystość aury', 'Bloom Radius': 'Promień blasku', 'Bloom Strength': 'Siła blasku',
  'Bloom Threshold': 'Próg blasku', 'Bone Thickness': 'Grubość kości', 'Chromatic Aberration': 'Aberracja chromatyczna',
  'Cinematic': 'Kinowy', 'Exposure': 'Ekspozycja', 'Film Grain': 'Ziarno filmowe', 'Floor Reflection': 'Odbicie podłogi',
  'FOV': 'FOV', 'Glow Intensity': 'Intensywność poświaty', 'Joint Color': 'Kolor stawów', 'Joint Size': 'Rozmiar stawów',
  'Minimal / Clean': 'Minimalny / czysty', 'Neon Glow': 'Neonowa poświata', 'Orbit Speed': 'Prędkość orbity',
  'Particle Trail': 'Ślad cząsteczek', 'Room Brightness': 'Jasność pokoju', 'Tactical / Military': 'Taktyczny / militarny',
  'Vignette': 'Winieta', 'Wireframe': 'Szkielet', 'Wireframe Color': 'Kolor szkieletu', 'Foundation (Default)': 'Podstawowy (domyślny)',
  // Dodatkowe etykiety HUD (wielkie nazwy scenariuszy / opisy / odznaki / status)
  'EMPTY ROOM': 'PUSTY POKÓJ', 'VITAL SIGNS': 'PARAMETRY ŻYCIOWE', 'MULTI-PERSON': 'WIELE OSÓB',
  'FALL DETECT': 'WYKRYWANIE UPADKU', 'SLEEP MONITOR': 'MONITOR SNU', 'INTRUSION': 'WŁAMANIE',
  'GESTURE CTRL': 'STEROWANIE GESTAMI', 'CROWD OCCUPANCY': 'ZAJĘTOŚĆ TŁUMU', 'SEARCH RESCUE': 'POSZUKIWANIE I RATUNEK',
  'ELDERLY CARE': 'OPIEKA NAD SENIORAMI', 'FITNESS': 'FITNESS', 'SECURITY PATROL': 'PATROL OCHRONY',
  'Auto-cycling through all sensing scenarios.': 'Automatyczne przełączanie wszystkich scenariuszy detekcji.',
  'Baseline calibration with no human presence in the monitored zone.': 'Kalibracja bazowa bez obecności człowieka w monitorowanej strefie.',
  'Detecting vital signs through WiFi signal micro-variations.': 'Wykrywanie parametrów życiowych poprzez mikrowariancje sygnału WiFi.',
  'Tracking multiple people simultaneously via CSI multiplex separation.': 'Śledzenie wielu osób jednocześnie dzięki separacji multipleksowej CSI.',
  'Sudden posture-change detection using acceleration feature analysis.': 'Wykrywanie nagłych zmian postawy za pomocą analizy cech przyspieszenia.',
  'Monitoring breathing patterns and apnea events during sleep.': 'Monitorowanie wzorców oddechu i epizodów bezdechu podczas snu.',
  'Passive perimeter monitoring -- no cameras, pure RF sensing.': 'Pasywny monitoring perymetru — bez kamer, czysta detekcja RF.',
  'DTW-based gesture recognition from hand/arm motion signatures.': 'Rozpoznawanie gestów w oparciu o DTW z sygnatur ruchu dłoni/ramion.',
  'Estimating room occupancy count from aggregate CSI variance.': 'Szacowanie liczby osób w pomieszczeniu na podstawie zagregowanej wariancji CSI.',
  'Through-wall survivor detection using WiFi-MAT multistatic mode.': 'Wykrywanie ocalałych przez ściany w trybie wielostatycznym WiFi-MAT.',
  'Continuous gait analysis for early mobility-decline detection.': 'Ciągła analiza chodu do wczesnego wykrywania pogorszenia sprawności ruchowej.',
  'Rep counting and exercise classification from body kinematics.': 'Liczenie powtórzeń i klasyfikacja ćwiczeń na podstawie kinematyki ciała.',
  'Multi-zone presence patrol with camera-free motion heatmaps.': 'Patrol obecności w wielu strefach z mapami cieplnymi ruchu bez kamer.',
  'VITALS': 'PARAMETRY', 'GAIT': 'CHÓD', 'TRACKING': 'ŚLEDZENIE', 'FALL': 'UPADEK',
  'APNEA': 'BEZDECH', 'PRESENCE': 'OBECNOŚĆ', 'ALERT': 'ALARM', 'GESTURE': 'GEST', 'OCCUPANCY': 'ZAJĘTOŚĆ',
  'LIVE': 'NA ŻYWO', 'DEMO': 'DEMO', 'ACTIVE': 'AKTYWNY', 'PRESENT': 'OBECNY',
};
const DICTS = { zh: ZH, pl: PL };

const SKIP = new Set(['SCRIPT', 'STYLE', 'TEXTAREA', 'CANVAS', 'NOSCRIPT']);
const originals = new WeakMap();          // Node -> 原始文本/属性值

function table(locale) { return DICTS[locale] || null; }

function restoreTextNode(n) {
  if (originals.has(n)) { n.nodeValue = originals.get(n); originals.delete(n); }
}

function translateTextNodes(root, tab) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(n) {
      if (!n.parentElement || SKIP.has(n.parentElement.tagName)) return NodeFilter.FILTER_REJECT;
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  let node;
  while ((node = walker.nextNode())) {
    const raw = node.nodeValue;
    const trimmed = raw.trim();
    if (!trimmed) continue;
    if (tab) {
      const hit = tab[trimmed];
      if (hit && hit !== trimmed) {
        if (!originals.has(node)) originals.set(node, raw);
        // 保留前后空白
        const lead = raw.slice(0, raw.indexOf(trimmed[0]));
        const trail = raw.slice(raw.lastIndexOf(trimmed[trimmed.length - 1]) + trimmed.length);
        node.nodeValue = lead + hit + trail;
      }
    } else {
      restoreTextNode(node);
    }
  }
}

const attrOriginals = new WeakMap();       // Element -> { attr: oryginalna wartość }

function translateAttrs(tab) {
  document.querySelectorAll('[title],[placeholder],[aria-label]').forEach(el => {
    for (const attr of ['title', 'placeholder', 'aria-label']) {
      const v = el.getAttribute(attr);
      if (v == null) continue;
      const store = attrOriginals.get(el) || {};
      if (tab && tab[v.trim()]) {
        if (!(attr in store)) { store[attr] = v; attrOriginals.set(el, store); }
        el.setAttribute(attr, tab[v.trim()]);
      } else if (attr in store) {
        el.setAttribute(attr, store[attr]);
        delete store[attr];
      }
    }
  });
}

function apply(locale) {
  const tab = table(locale);
  translateTextNodes(document.body, tab);
  translateAttrs(tab);
}

// 语言下拉（observatory 原本没有）：注入右上角，复用共享 i18n 的持久化
function ensureSelector() {
  if (document.getElementById('obs-lang')) return;
  const sel = document.createElement('select');
  sel.id = 'obs-lang';
  sel.setAttribute('aria-label', 'Language');
  sel.style.cssText = 'position:fixed;top:10px;right:12px;z-index:9999;background:#12151c;color:#e6edf3;border:1px solid #2a3140;border-radius:8px;padding:4px 8px;font:13px system-ui';
  sel.innerHTML = '<option value="en">EN</option><option value="pl">PL</option><option value="zh">中文</option>';
  sel.value = i18n.locale;
  sel.addEventListener('change', () => i18n.setLocale(sel.value));
  document.body.appendChild(sel);
}

let busy = false, mo = null;
function scheduleApply() {                 // 去抖：动态文案变了就重扫
  if (busy) return;
  busy = true;
  requestAnimationFrame(() => { try { apply(i18n.locale); } finally { busy = false; } });
}

function start() {
  ensureSelector();
  apply(i18n.locale);
  i18n.onLocaleChange(l => { const s = document.getElementById('obs-lang'); if (s) s.value = l; apply(l); });
  if (mo) mo.disconnect();
  mo = new MutationObserver(() => { if (mo) { mo.takeRecords(); } scheduleApply(); });
  mo.observe(document.body, { childList: true, subtree: true, characterData: true });
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
else start();

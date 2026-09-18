// Internationalization - EN/PL/zh-CN language support
// Detects browser language, persists choice, translates UI strings

const translations = {
  en: {
    // Navigation
    'nav.dashboard': 'Dashboard',
    'nav.hardware': 'Hardware',
    'nav.demo': 'Live Demo',
    'nav.architecture': 'Architecture',
    'nav.performance': 'Performance',
    'nav.applications': 'Applications',
    'nav.sensing': 'Sensing',
    'nav.training': 'Training',

    // Dashboard
    'dashboard.title': 'Revolutionary WiFi-Based Human Pose Detection',
    'dashboard.subtitle': 'Human Tracking Through Walls Using WiFi Signals',
    'dashboard.description': 'AI can track your full-body movement through walls using just WiFi signals. Researchers at Carnegie Mellon have trained a neural network to turn basic WiFi signals into detailed wireframe models of human bodies.',
    'dashboard.status': 'System Status',
    'dashboard.metrics': 'System Metrics',
    'dashboard.features': 'Features',
    'dashboard.liveStats': 'Live Statistics',
    'dashboard.activePersons': 'Active Persons',
    'dashboard.avgConfidence': 'Avg Confidence',
    'dashboard.totalDetections': 'Total Detections',
    'dashboard.zoneOccupancy': 'Zone Occupancy',

    // Status
    'status.apiServer': 'API Server',
    'status.hardware': 'Hardware',
    'status.inference': 'Inference',
    'status.streaming': 'Streaming',
    'status.dataSource': 'Data Source',

    // Metrics
    'metrics.cpu': 'CPU Usage',
    'metrics.memory': 'Memory Usage',
    'metrics.disk': 'Disk Usage',

    // Benefits
    'benefit.throughWalls': 'Through Walls',
    'benefit.throughWallsDesc': 'Works through solid barriers with no line of sight required',
    'benefit.privacy': 'Privacy-Preserving',
    'benefit.privacyDesc': 'No cameras or visual recording - just WiFi signal analysis',
    'benefit.realtime': 'Real-Time',
    'benefit.realtimeDesc': 'Maps 24 body regions in real-time at 100Hz sampling rate',
    'benefit.lowCost': 'Low Cost',
    'benefit.lowCostDesc': 'Built using $30 commercial WiFi hardware',

    // Stats
    'stat.bodyRegions': 'Body Regions',
    'stat.samplingRate': 'Sampling Rate',
    'stat.accuracy': 'Accuracy (AP@50)',
    'stat.hardwareCost': 'Hardware Cost',

    // Actions
    'action.startDetection': 'Start Detection',
    'action.stopDetection': 'Stop Detection',
    'action.toggleTheme': 'Toggle theme',
    'action.exportData': 'Export data',
    'action.screenshot': 'Take screenshot',

    // Connection
    'conn.connected': 'Connected',
    'conn.connecting': 'Connecting...',
    'conn.offline': 'Offline',
    'conn.reconnecting': 'Reconnecting...',
    'conn.live': 'Live',
    'conn.simulated': 'Simulated',

    // Misc
    'misc.loading': 'Loading...',
    'misc.error': 'An error occurred',
    'misc.noData': 'No data available',
    'misc.close': 'Close',
    'misc.cancel': 'Cancel',
    'misc.confirm': 'Confirm',
    'misc.settings': 'Settings',
    'misc.language': 'Language',

    // --- i18n zh coverage pass (2026-09): static shell + app messages ---
    // Navigation (extra)
    'nav.posefusion': 'Pose Fusion',
    'nav.observatory': 'Observatory',

    // Accessibility / shell
    'a11y.skipToContent': 'Skip to main content',
    'a11y.mainNav': 'Main navigation',
    'a11y.apiVersion': 'API version',
    'a11y.environment': 'Environment',
    'a11y.systemHealth': 'System health',

    // Hero
    'dashboard.heroDesc': 'AI can track your full-body movement through walls using just WiFi signals. Researchers at Carnegie Mellon have trained a neural network to turn basic WiFi signals into detailed wireframe models of human bodies.',

    // Hardware
    'hardware.title': 'Hardware Configuration',
    'hardware.antennaArray': '3\u00d73 Antenna Array',
    'hardware.antennaHelp': 'Click antennas to toggle their state',
    'hardware.transmitters': 'Transmitters (3)',
    'hardware.receivers': 'Receivers (6)',
    'hardware.wifiConfig': 'WiFi Configuration',
    'hardware.frequency': 'Frequency',
    'hardware.subcarriers': 'Subcarriers',
    'hardware.samplingRate': 'Sampling Rate',
    'hardware.totalCost': 'Total Cost',
    'hardware.csiData': 'Real-time CSI Data',
    'hardware.amplitude': 'Amplitude:',
    'hardware.phase': 'Phase:',

    // Demo
    'demo.title': 'Live Demonstration',
    'demo.startStream': 'Start Stream',
    'demo.stopStream': 'Stop Stream',
    'demo.ready': 'Ready',
    'demo.signalAnalysis': 'WiFi Signal Analysis',
    'demo.signalStrength': 'Signal Strength:',
    'demo.latency': 'Processing Latency:',
    'demo.poseDetection': 'Human Pose Detection',
    'demo.personsDetected': 'Persons Detected:',
    'demo.confidence': 'Confidence:',
    'demo.keypoints': 'Keypoints:',

    // Architecture
    'arch.title': 'System Architecture',
    'arch.step1Title': 'CSI Input',
    'arch.step1Text': 'Channel State Information collected from WiFi antenna array',
    'arch.step2Title': 'Phase Sanitization',
    'arch.step2Text': 'Remove hardware-specific noise and normalize signal phase',
    'arch.step3Title': 'Modality Translation',
    'arch.step3Text': 'Convert WiFi signals to visual representation using CNN',
    'arch.step4Title': 'DensePose-RCNN',
    'arch.step4Text': 'Extract human pose keypoints and body part segmentation',
    'arch.step5Title': 'Wireframe Output',
    'arch.step5Text': 'Generate final human pose wireframe visualization',

    // Performance
    'perf.title': 'Performance Analysis',
    'perf.wifiBased': 'WiFi-based (Same Layout)',
    'perf.imageBased': 'Image-based (Reference)',
    'perf.avgPrecision': 'Average Precision:',
    'perf.advantages': 'Advantages & Limitations',
    'perf.advantagesTitle': 'Advantages',
    'perf.limitationsTitle': 'Limitations',
    'perf.adv.throughWall': 'Through-wall detection',
    'perf.adv.privacy': 'Privacy preserving',
    'perf.adv.lighting': 'Lighting independent',
    'perf.adv.lowCost': 'Low cost hardware',
    'perf.adv.existingWifi': 'Uses existing WiFi',
    'perf.lim.layout': 'Performance drops in different layouts',
    'perf.lim.devices': 'Requires WiFi-compatible devices',
    'perf.lim.data': 'Training requires synchronized data',

    // Applications
    'apps.title': 'Real-World Applications',
    'apps.elderly.title': 'Elderly Care Monitoring',
    'apps.elderly.desc': 'Monitor elderly individuals for falls or emergencies without invading privacy. Track movement patterns and detect anomalies in daily routines.',
    'apps.elderly.tag1': 'Fall Detection',
    'apps.elderly.tag2': 'Activity Monitoring',
    'apps.elderly.tag3': 'Emergency Alert',
    'apps.security.title': 'Home Security Systems',
    'apps.security.desc': 'Detect intruders and monitor home security without visible cameras. Track multiple persons and identify suspicious movement patterns.',
    'apps.security.tag1': 'Intrusion Detection',
    'apps.security.tag2': 'Multi-person Tracking',
    'apps.security.tag3': 'Invisible Monitoring',
    'apps.healthcare.title': 'Healthcare Patient Monitoring',
    'apps.healthcare.desc': 'Monitor patients in hospitals and care facilities. Track vital signs through movement analysis and detect health emergencies.',
    'apps.healthcare.tag1': 'Vital Sign Analysis',
    'apps.healthcare.tag2': 'Movement Tracking',
    'apps.healthcare.tag3': 'Health Alerts',
    'apps.building.title': 'Smart Building Occupancy',
    'apps.building.desc': 'Optimize building energy consumption by tracking occupancy patterns. Control lighting, HVAC, and security systems automatically.',
    'apps.building.tag1': 'Energy Optimization',
    'apps.building.tag2': 'Occupancy Tracking',
    'apps.building.tag3': 'Smart Controls',
    'apps.arvr.title': 'AR/VR Applications',
    'apps.arvr.desc': 'Enable full-body tracking for virtual and augmented reality applications without wearing additional sensors or cameras.',
    'apps.arvr.tag1': 'Full Body Tracking',
    'apps.arvr.tag2': 'Sensor-free',
    'apps.arvr.tag3': 'Immersive Experience',
    'apps.implTitle': 'Implementation Considerations',
    'apps.implText': 'While WiFi DensePose offers revolutionary capabilities, successful implementation requires careful consideration of environment setup, data privacy regulations, and system calibration for optimal performance.',

    // Training
    'training.title': 'Model Training',
    'training.subtitle': 'Record CSI data, train pose estimation models, and manage .rvf files',

    // App-level messages
    'app.initFailed': 'Failed to initialize application. Please refresh the page.',
    'app.unexpectedError': 'An unexpected error occurred',
    'app.mockActive': 'Mock server active - testing mode',
    'app.connectedRust': 'Connected to Rust sensing server',
    'app.backendUnavailable': 'Backend unavailable \u2014 start sensing-server',

    // --- canvas / 3D sprite text (2026-09) ---
    'viz.csiAmplitude': 'CSI AMPLITUDE',
    'viz.phase': 'PHASE',
    'viz.dopplerSpectrum': 'DOPPLER SPECTRUM',
    'viz.motion': 'MOTION',
    'viz.zone1': 'Zone 1',
    'viz.zone2': 'Zone 2',
    'viz.zone3': 'Zone 3',
    'canvas.waitingCsi': 'Waiting for CSI data...',
    'canvas.joints': 'joints',
    'canvas.subcarrierAxis': 'Subcarrier →',
    'canvas.timeAxis': 'Time ↑',
    'canvas.video': 'Video',
    'canvas.fused': 'Fused',
    'training.noData': 'No data',

  },

  pl: {
    // Nawigacja
    'nav.dashboard': 'Panel główny',
    'nav.hardware': 'Sprzęt',
    'nav.demo': 'Demo na żywo',
    'nav.architecture': 'Architektura',
    'nav.performance': 'Wydajność',
    'nav.applications': 'Aplikacje',
    'nav.sensing': 'Czujniki',
    'nav.training': 'Trening',
    'nav.posefusion': 'Fuzja pozycji',
    'nav.observatory': 'Obserwatorium',

    // Pulpit
    'dashboard.title': 'Rewolucyjne wykrywanie pozycji człowieka przez WiFi',
    'dashboard.subtitle': 'Śledzenie ludzi przez ściany za pomocą sygnałów WiFi',
    'dashboard.description': 'AI może śledzić ruchy całego ciała przez ściany, używając wyłącznie sygnałów WiFi. Naukowcy z Carnegie Mellon wytrenowali sieć neuronową, która zamienia zwykłe sygnały WiFi w szczegółowe modele szkieletowe ludzkiego ciała.',
    'dashboard.heroDesc': 'AI może śledzić ruchy całego ciała przez ściany, używając wyłącznie sygnałów WiFi. Naukowcy z Carnegie Mellon wytrenowali sieć neuronową, która zamienia zwykłe sygnały WiFi w szczegółowe modele szkieletowe ludzkiego ciała.',
    'dashboard.status': 'Stan systemu',
    'dashboard.metrics': 'Metryki systemu',
    'dashboard.features': 'Funkcje',
    'dashboard.liveStats': 'Statystyki na żywo',
    'dashboard.activePersons': 'Aktywne osoby',
    'dashboard.avgConfidence': 'Średnia pewność',
    'dashboard.totalDetections': 'Łączne detekcje',
    'dashboard.zoneOccupancy': 'Zajętość stref',

    // Status
    'status.apiServer': 'Serwer API',
    'status.hardware': 'Sprzęt',
    'status.inference': 'Wnioskowanie',
    'status.streaming': 'Strumieniowanie',
    'status.dataSource': 'Źródło danych',

    // Metryki
    'metrics.cpu': 'Użycie CPU',
    'metrics.memory': 'Użycie pamięci',
    'metrics.disk': 'Użycie dysku',

    // Zalety
    'benefit.throughWalls': 'Przez ściany',
    'benefit.throughWallsDesc': 'Działa przez stałe przeszkody bez linii wzroku',
    'benefit.privacy': 'Ochrona prywatności',
    'benefit.privacyDesc': 'Bez kamer i nagrywania obrazu — tylko analiza sygnałów WiFi',
    'benefit.realtime': 'Czas rzeczywisty',
    'benefit.realtimeDesc': 'Odwzorowuje 24 regiony ciała w czasie rzeczywistym z próbkowaniem 100 Hz',
    'benefit.lowCost': 'Niski koszt',
    'benefit.lowCostDesc': 'Zbudowany z komercyjnego sprzętu WiFi za 30 USD',

    // Statystyki
    'stat.bodyRegions': 'Regiony ciała',
    'stat.samplingRate': 'Częstotliwość próbkowania',
    'stat.accuracy': 'Dokładność (AP@50)',
    'stat.hardwareCost': 'Koszt sprzętu',

    // Akcje
    'action.startDetection': 'Rozpocznij detekcję',
    'action.stopDetection': 'Zatrzymaj detekcję',
    'action.toggleTheme': 'Przełącz motyw',
    'action.exportData': 'Eksportuj dane',
    'action.screenshot': 'Zrób zrzut ekranu',

    // Połączenie
    'conn.connected': 'Połączono',
    'conn.connecting': 'Łączenie...',
    'conn.offline': 'Offline',
    'conn.reconnecting': 'Ponowne łączenie...',
    'conn.live': 'Na żywo',
    'conn.simulated': 'Symulacja',

    // Różne
    'misc.loading': 'Ładowanie...',
    'misc.error': 'Wystąpił błąd',
    'misc.noData': 'Brak danych',
    'misc.close': 'Zamknij',
    'misc.cancel': 'Anuluj',
    'misc.confirm': 'Potwierdź',
    'misc.settings': 'Ustawienia',
    'misc.language': 'Język',

    // Dostępność / powłoka
    'a11y.skipToContent': 'Przejdź do treści głównej',
    'a11y.mainNav': 'Nawigacja główna',
    'a11y.apiVersion': 'Wersja API',
    'a11y.environment': 'Środowisko',
    'a11y.systemHealth': 'Stan systemu',

    // Sprzęt
    'hardware.title': 'Konfiguracja sprzętu',
    'hardware.antennaArray': 'Układ anten 3×3',
    'hardware.antennaHelp': 'Kliknij anteny, aby przełączyć ich stan',
    'hardware.transmitters': 'Nadajniki (3)',
    'hardware.receivers': 'Odbiorniki (6)',
    'hardware.wifiConfig': 'Konfiguracja WiFi',
    'hardware.frequency': 'Częstotliwość',
    'hardware.subcarriers': 'Podnośne',
    'hardware.samplingRate': 'Częstotliwość próbkowania',
    'hardware.totalCost': 'Koszt całkowity',
    'hardware.csiData': 'Dane CSI w czasie rzeczywistym',
    'hardware.amplitude': 'Amplituda:',
    'hardware.phase': 'Faza:',

    // Demo
    'demo.title': 'Demonstracja na żywo',
    'demo.startStream': 'Uruchom strumień',
    'demo.stopStream': 'Zatrzymaj strumień',
    'demo.ready': 'Gotowe',
    'demo.signalAnalysis': 'Analiza sygnału WiFi',
    'demo.signalStrength': 'Siła sygnału:',
    'demo.latency': 'Opóźnienie przetwarzania:',
    'demo.poseDetection': 'Wykrywanie pozycji człowieka',
    'demo.personsDetected': 'Wykryte osoby:',
    'demo.confidence': 'Pewność:',
    'demo.keypoints': 'Punkty kluczowe:',

    // Architektura
    'arch.title': 'Architektura systemu',
    'arch.step1Title': 'Wejście CSI',
    'arch.step1Text': 'Informacja o stanie kanału (CSI) zbierana z układu anten WiFi',
    'arch.step2Title': 'Oczyszczanie fazy',
    'arch.step2Text': 'Usuwa szumy sprzętowe i normalizuje fazę sygnału',
    'arch.step3Title': 'Translacja modalności',
    'arch.step3Text': 'Konwersja sygnałów WiFi na reprezentację wizualną za pomocą CNN',
    'arch.step4Title': 'DensePose-RCNN',
    'arch.step4Text': 'Ekstrakcja punktów kluczowych pozycji i segmentacja części ciała',
    'arch.step5Title': 'Wyjście szkieletowe',
    'arch.step5Text': 'Generowanie końcowej wizualizacji szkieletu pozycji człowieka',

    // Wydajność
    'perf.title': 'Analiza wydajności',
    'perf.wifiBased': 'WiFi (ten sam układ)',
    'perf.imageBased': 'Obrazowo (odniesienie)',
    'perf.avgPrecision': 'Średnia precyzja:',
    'perf.advantages': 'Zalety i ograniczenia',
    'perf.advantagesTitle': 'Zalety',
    'perf.limitationsTitle': 'Ograniczenia',
    'perf.adv.throughWall': 'Wykrywanie przez ściany',
    'perf.adv.privacy': 'Ochrona prywatności',
    'perf.adv.lighting': 'Niezależność od oświetlenia',
    'perf.adv.lowCost': 'Tani sprzęt',
    'perf.adv.existingWifi': 'Wykorzystuje istniejące WiFi',
    'perf.lim.layout': 'Wydajność spada przy innym układzie pomieszczeń',
    'perf.lim.devices': 'Wymaga urządzeń zgodnych z WiFi',
    'perf.lim.data': 'Trening wymaga zsynchronizowanych danych',

    // Aplikacje
    'apps.title': 'Zastosowania w praktyce',
    'apps.elderly.title': 'Monitoring opieki nad seniorami',
    'apps.elderly.desc': 'Monitorowanie osób starszych pod kątem upadków i sytuacji awaryjnych bez naruszania prywatności. Śledzenie wzorców ruchu i wykrywanie anomalii w codziennych czynnościach.',
    'apps.elderly.tag1': 'Wykrywanie upadków',
    'apps.elderly.tag2': 'Monitoring aktywności',
    'apps.elderly.tag3': 'Alerty awaryjne',
    'apps.security.title': 'Systemy bezpieczeństwa domu',
    'apps.security.desc': 'Wykrywanie intruzów i ochrona domu bez widocznych kamer. Śledzenie wielu osób i identyfikacja podejrzanych wzorców ruchu.',
    'apps.security.tag1': 'Wykrywanie włamań',
    'apps.security.tag2': 'Śledzenie wielu osób',
    'apps.security.tag3': 'Niewidzialny monitoring',
    'apps.healthcare.title': 'Monitorowanie pacjentów w służbie zdrowia',
    'apps.healthcare.desc': 'Monitorowanie pacjentów w szpitalach i placówkach opiekuńczych. Analiza parametrów życiowych na podstawie ruchu i wykrywanie zagrożeń zdrowotnych.',
    'apps.healthcare.tag1': 'Analiza parametrów życiowych',
    'apps.healthcare.tag2': 'Śledzenie ruchu',
    'apps.healthcare.tag3': 'Alerty zdrowotne',
    'apps.building.title': 'Zajętość inteligentnych budynków',
    'apps.building.desc': 'Optymalizacja zużycia energii budynku dzięki śledzeniu zajętości. Automatyczne sterowanie oświetleniem, HVAC i systemami bezpieczeństwa.',
    'apps.building.tag1': 'Optymalizacja energii',
    'apps.building.tag2': 'Śledzenie zajętości',
    'apps.building.tag3': 'Inteligentne sterowanie',
    'apps.arvr.title': 'Zastosowania AR/VR',
    'apps.arvr.desc': 'Pełne śledzenie ciała dla rzeczywistości wirtualnej i rozszerzonej bez dodatkowych czujników i kamer.',
    'apps.arvr.tag1': 'Pełne śledzenie ciała',
    'apps.arvr.tag2': 'Bez czujników',
    'apps.arvr.tag3': 'Immersyjne doświadczenie',
    'apps.implTitle': 'Uwarunkowania wdrożenia',
    'apps.implText': 'Choć WiFi DensePose oferuje rewolucyjne możliwości, skuteczne wdrożenie wymaga starannego przygotowania środowiska, zgodności z przepisami o ochronie danych oraz kalibracji systemu dla osiągnięcia optymalnej wydajności.',

    // Trening
    'training.title': 'Trenowanie modelu',
    'training.subtitle': 'Nagrywaj dane CSI, trenuj modele estymacji pozycji i zarządzaj plikami .rvf',

    // Komunikaty aplikacji
    'app.initFailed': 'Inicjalizacja aplikacji nie powiodła się. Odśwież stronę.',
    'app.unexpectedError': 'Wystąpił nieoczekiwany błąd',
    'app.mockActive': 'Serwer testowy aktywny — tryb testowy',
    'app.connectedRust': 'Połączono z serwerem pomiarowym Rust',
    'app.backendUnavailable': 'Backend niedostępny — uruchom sensing-server',
    'viz.csiAmplitude': 'AMPLITUDA CSI',
    'viz.phase': 'FAZA',
    'viz.dopplerSpectrum': 'WIDMO DOPPLERA',
    'viz.motion': 'RUCH',
    'viz.zone1': 'Strefa 1',
    'viz.zone2': 'Strefa 2',
    'viz.zone3': 'Strefa 3',
    'canvas.waitingCsi': 'Oczekiwanie na dane CSI...',
    'canvas.joints': 'stawów',
    'canvas.subcarrierAxis': 'Podnośne →',
    'canvas.timeAxis': 'Czas ↑',
    'canvas.video': 'Wideo',
    'canvas.fused': 'Połączone',
    'training.noData': 'Brak danych',
  },

  zh: {
    // 导航
    'nav.dashboard': '仪表板',
    'nav.hardware': '硬件',
    'nav.demo': '实时演示',
    'nav.architecture': '架构',
    'nav.performance': '性能',
    'nav.applications': '应用',
    'nav.sensing': '感知',
    'nav.training': '训练',

    // 仪表板
    'dashboard.title': '革命性的 WiFi 人体姿态检测',
    'dashboard.subtitle': '利用 WiFi 信号实现穿墙人体追踪',
    'dashboard.description': 'AI 仅凭 WiFi 信号即可穿透墙壁追踪你的全身动作。卡内基梅隆大学的研究人员训练了一个神经网络，把普通 WiFi 信号转换为精细的人体线框模型。',
    'dashboard.status': '系统状态',
    'dashboard.metrics': '系统指标',
    'dashboard.features': '功能特性',
    'dashboard.liveStats': '实时统计',
    'dashboard.activePersons': '活动人数',
    'dashboard.avgConfidence': '平均置信度',
    'dashboard.totalDetections': '总检测数',
    'dashboard.zoneOccupancy': '区域占用',

    // 状态
    'status.apiServer': 'API 服务器',
    'status.hardware': '硬件',
    'status.inference': '推理',
    'status.streaming': '数据流',
    'status.dataSource': '数据源',

    // 指标
    'metrics.cpu': 'CPU 使用率',
    'metrics.memory': '内存使用率',
    'metrics.disk': '磁盘使用率',

    // 优势
    'benefit.throughWalls': '穿墙感知',
    'benefit.throughWallsDesc': '无需视线即可穿透固体障碍工作',
    'benefit.privacy': '保护隐私',
    'benefit.privacyDesc': '无摄像头、无画面录制，仅分析 WiFi 信号',
    'benefit.realtime': '实时',
    'benefit.realtimeDesc': '以 100Hz 采样率实时映射 24 个身体区域',
    'benefit.lowCost': '低成本',
    'benefit.lowCostDesc': '基于 30 美元的商用 WiFi 硬件构建',

    // 规格
    'stat.bodyRegions': '身体区域',
    'stat.samplingRate': '采样率',
    'stat.accuracy': '准确率 (AP@50)',
    'stat.hardwareCost': '硬件成本',

    // 操作
    'action.startDetection': '开始检测',
    'action.stopDetection': '停止检测',
    'action.toggleTheme': '切换主题',
    'action.exportData': '导出数据',
    'action.screenshot': '截图',

    // 连接
    'conn.connected': '已连接',
    'conn.connecting': '连接中…',
    'conn.offline': '离线',
    'conn.reconnecting': '重新连接中…',
    'conn.live': '实时',
    'conn.simulated': '模拟',

    // 其它
    'misc.loading': '加载中…',
    'misc.error': '发生错误',
    'misc.noData': '暂无数据',
    'misc.close': '关闭',
    'misc.cancel': '取消',
    'misc.confirm': '确认',
    'misc.settings': '设置',
    'misc.language': '语言',

    // --- i18n zh coverage pass (2026-09): 静态壳层 + 应用消息 ---
    'nav.posefusion': '态势融合',
    'nav.observatory': '观测台',

    'a11y.skipToContent': '跳转到主内容',
    'a11y.mainNav': '主导航',
    'a11y.apiVersion': 'API 版本',
    'a11y.environment': '环境',
    'a11y.systemHealth': '系统健康',

    'dashboard.heroDesc': 'AI 仅凭 WiFi 信号即可穿墙追踪你的全身动作。卡内基梅隆大学的研究人员训练了一个神经网络，能把普通 WiFi 信号转换为精细的人体线框模型。',

    'hardware.title': '硬件配置',
    'hardware.antennaArray': '3×3 天线阵列',
    'hardware.antennaHelp': '点击天线切换其收发状态',
    'hardware.transmitters': '发射端（3）',
    'hardware.receivers': '接收端（6）',
    'hardware.wifiConfig': 'WiFi 配置',
    'hardware.frequency': '频率',
    'hardware.subcarriers': '子载波数',
    'hardware.samplingRate': '采样率',
    'hardware.totalCost': '总成本',
    'hardware.csiData': '实时 CSI 数据',
    'hardware.amplitude': '幅度：',
    'hardware.phase': '相位：',

    'demo.title': '实时演示',
    'demo.startStream': '开始推流',
    'demo.stopStream': '停止推流',
    'demo.ready': '就绪',
    'demo.signalAnalysis': 'WiFi 信号分析',
    'demo.signalStrength': '信号强度：',
    'demo.latency': '处理延迟：',
    'demo.poseDetection': '人体姿态检测',
    'demo.personsDetected': '检测到人数：',
    'demo.confidence': '置信度：',
    'demo.keypoints': '关键点：',

    'arch.title': '系统架构',
    'arch.step1Title': 'CSI 输入',
    'arch.step1Text': '从 WiFi 天线阵列采集信道状态信息（CSI）',
    'arch.step2Title': '相位净化',
    'arch.step2Text': '去除硬件相关噪声并归一化信号相位',
    'arch.step3Title': '模态转换',
    'arch.step3Text': '使用 CNN 将 WiFi 信号转换为视觉表征',
    'arch.step4Title': 'DensePose-RCNN',
    'arch.step4Text': '提取人体姿态关键点与身体部位分割',
    'arch.step5Title': '线框输出',
    'arch.step5Text': '生成最终的人体姿态线框可视化',

    'perf.title': '性能分析',
    'perf.wifiBased': 'WiFi 方案（同布局）',
    'perf.imageBased': '图像方案（参照）',
    'perf.avgPrecision': '平均精度：',
    'perf.advantages': '优势与局限',
    'perf.advantagesTitle': '优势',
    'perf.limitationsTitle': '局限',
    'perf.adv.throughWall': '穿墙检测',
    'perf.adv.privacy': '保护隐私',
    'perf.adv.lighting': '不受光照影响',
    'perf.adv.lowCost': '硬件成本低',
    'perf.adv.existingWifi': '复用现有 WiFi',
    'perf.lim.layout': '跨布局场景性能会下降',
    'perf.lim.devices': '需要兼容 WiFi 的设备',
    'perf.lim.data': '训练需要同步采集的数据',

    'apps.title': '实际应用场景',
    'apps.elderly.title': '养老照护监测',
    'apps.elderly.desc': '在不侵犯隐私的前提下监护老年人的跌倒与突发状况，追踪活动规律并发现日常行为异常。',
    'apps.elderly.tag1': '跌倒检测',
    'apps.elderly.tag2': '活动监测',
    'apps.elderly.tag3': '紧急告警',
    'apps.security.title': '家庭安防系统',
    'apps.security.desc': '无需可见摄像头即可发现入侵者并守护家庭安全，支持多人追踪与可疑行为识别。',
    'apps.security.tag1': '入侵检测',
    'apps.security.tag2': '多人追踪',
    'apps.security.tag3': '无感监控',
    'apps.healthcare.title': '医疗患者监测',
    'apps.healthcare.desc': '面向医院与护理机构的患者监护，通过运动分析追踪生命体征并及时发现健康急症。',
    'apps.healthcare.tag1': '生命体征分析',
    'apps.healthcare.tag2': '动作追踪',
    'apps.healthcare.tag3': '健康预警',
    'apps.building.title': '智慧楼宇占用分析',
    'apps.building.desc': '通过占用规律追踪优化楼宇能耗，自动联动照明、暖通与安防系统。',
    'apps.building.tag1': '能耗优化',
    'apps.building.tag2': '占用追踪',
    'apps.building.tag3': '智能联动控制',
    'apps.arvr.title': 'AR/VR 应用',
    'apps.arvr.desc': '为虚拟/增强现实提供全身追踪，无需穿戴额外传感器或摄像头。',
    'apps.arvr.tag1': '全身追踪',
    'apps.arvr.tag2': '免传感器',
    'apps.arvr.tag3': '沉浸式体验',
    'apps.implTitle': '落地实施要点',
    'apps.implText': 'WiFi DensePose 能力突破性强，但要成功落地，仍需周全考虑环境布置、数据隐私合规以及系统校准，才能达到最佳效果。',

    'training.title': '模型训练',
    'training.subtitle': '录制 CSI 数据、训练姿态估计模型并管理 .rvf 文件',

    'app.initFailed': '应用初始化失败，请刷新页面。',
    'app.unexpectedError': '发生意外错误',
    'app.mockActive': '模拟服务器运行中 - 测试模式',
    'app.connectedRust': '已连接 Rust 感知服务器',
    'app.backendUnavailable': '后端不可用 — 请启动 sensing-server',

    'viz.csiAmplitude': 'CSI 幅度',
    'viz.phase': '相位',
    'viz.dopplerSpectrum': '多普勒频谱',
    'viz.motion': '运动',
    'viz.zone1': '区域 1',
    'viz.zone2': '区域 2',
    'viz.zone3': '区域 3',
    'canvas.waitingCsi': '等待 CSI 数据...',
    'canvas.joints': '个关节',
    'canvas.subcarrierAxis': '子载波 →',
    'canvas.timeAxis': '时间 ↑',
    'canvas.video': '视频',
    'canvas.fused': '融合',
    'training.noData': '暂无数据',
  }
};

export class I18n {
  constructor() {
    this.locale = this.getSavedLocale() || this.detectLocale();
    this.listeners = [];
  }

  init() {
    this.createSelector();
    document.documentElement.setAttribute('lang', this.locale);
    this.applyTranslations();
  }

  detectLocale() {
    const lang = navigator.language?.toLowerCase() || 'en';
    if (lang.startsWith('pl')) return 'pl';
    if (lang.startsWith('zh')) return 'zh';
    return 'en';
  }

  getSavedLocale() {
    try { return localStorage.getItem('ruview-locale'); }
    catch { return null; }
  }

  saveLocale(locale) {
    try { localStorage.setItem('ruview-locale', locale); }
    catch { /* noop */ }
  }

  t(key) {
    const dict = translations[this.locale] || translations.en;
    return dict[key] || translations.en[key] || key;
  }

  setLocale(locale) {
    if (!translations[locale]) return;
    this.locale = locale;
    this.saveLocale(locale);
    document.documentElement.setAttribute('lang', locale);
    this.applyTranslations();
    this.listeners.forEach(cb => { try { cb(locale); } catch { /* noop */ } });
  }

  onLocaleChange(callback) {
    this.listeners.push(callback);
    return () => {
      const i = this.listeners.indexOf(callback);
      if (i > -1) this.listeners.splice(i, 1);
    };
  }

  applyTranslations() {
    // Translate elements with data-i18n attribute
    document.querySelectorAll('[data-i18n]').forEach(el => {
      const key = el.getAttribute('data-i18n');
      el.textContent = this.t(key);
    });

    // Translate placeholders
    document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
      const key = el.getAttribute('data-i18n-placeholder');
      el.placeholder = this.t(key);
    });

    // Translate aria-labels
    document.querySelectorAll('[data-i18n-aria]').forEach(el => {
      const key = el.getAttribute('data-i18n-aria');
      el.setAttribute('aria-label', this.t(key));
    });

    // Update language selector
    const selector = document.getElementById('lang-selector');
    if (selector) selector.value = this.locale;
  }

  createSelector() {
    const wrapper = document.createElement('div');
    wrapper.className = 'lang-selector-wrap';
    wrapper.innerHTML = `
      <select id="lang-selector" class="lang-selector" aria-label="Language">
        <option value="en">EN</option>
        <option value="pl">PL</option>
        <option value="zh">简体中文</option>
      </select>
    `;

    const select = wrapper.querySelector('select');
    select.value = this.locale;
    select.addEventListener('change', () => this.setLocale(select.value));

    const headerInfo = document.querySelector('.header-info');
    if (headerInfo) {
      headerInfo.appendChild(wrapper);
    }
  }

  getAvailableLocales() {
    return Object.keys(translations);
  }

  dispose() {
    this.listeners = [];
  }
}

export const i18n = new I18n();

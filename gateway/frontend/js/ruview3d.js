/* RuView WiFi CSI 3D visualization for NovaSense Gateway — v3.
 * Mimics the official RuView GaussianSplatRenderer:
 *   - ShaderMaterial soft-disc splats with additive blending (glow)
 *   - signal_field (20x20) -> glowing heat floor, size/opacity driven by value
 *   - body blob (64 splats) per person with BREATHING PULSATION animation
 *   - colorful node markers, room grid, orbit controls
 * Plus NovaSense additions: posture label (stand/sit/lie/move), 60s replay.
 * Wire-up: window.__rv3d.update(msg) on every real frame.
 */
(function () {
  var diag = function (txt, color) {
    var d = document.getElementById('rv3dDiag');
    if (d) { if (color) d.style.color = color; d.textContent = txt; }
  };
  try {
  if (typeof THREE === 'undefined') { diag('THREE 未加载（/js/three.min.js 404？）', '#ffcc00'); return; }
  var container = document.getElementById('rv3d');
  if (!container) { diag('3D 容器 #rv3d 不存在', '#ffcc00'); return; }

  var W = container.clientWidth || 640;
  var H = container.clientHeight || 380;

  /* ---- custom splat shaders (from RuView gaussian-splats.js) ---- */
  var SPLAT_VERTEX = [
    'attribute float splatSize;',
    'attribute vec3  splatColor;',
    'attribute float splatOpacity;',
    'varying vec3  vColor;',
    'varying float vOpacity;',
    'void main() {',
    '  vColor   = splatColor;',
    '  vOpacity = splatOpacity;',
    '  vec4 mvPosition = modelViewMatrix * vec4(position, 1.0);',
    '  gl_PointSize = splatSize * (300.0 / -mvPosition.z);',
    '  gl_Position  = projectionMatrix * mvPosition;',
    '}'
  ].join('\n');
  var SPLAT_FRAGMENT = [
    'varying vec3  vColor;',
    'varying float vOpacity;',
    'void main() {',
    '  float dist = length(gl_PointCoord - vec2(0.5));',
    '  if (dist > 0.5) discard;',
    '  float alpha = smoothstep(0.5, 0.2, dist) * vOpacity;',
    '  gl_FragColor = vec4(vColor, alpha);',
    '}'
  ].join('\n');

  var scene = new THREE.Scene();
  scene.background = new THREE.Color(0x0a0a12);
  var camera = new THREE.PerspectiveCamera(55, W / H, 0.1, 200);
  camera.position.set(0, 14, 14);
  camera.lookAt(0, 0, 0);
  var renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
  renderer.setSize(W, H);
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  container.appendChild(renderer.domElement);

  var controls = new THREE.OrbitControls(camera, renderer.domElement);
  controls.target.set(0, 1.5, 0);
  controls.enableDamping = true;
  controls.maxPolarAngle = Math.PI / 2.05;

  /* ---- room: 20x20 grid + boundary wireframe (official layout) ---- */
  var grid = new THREE.GridHelper(20, 20, 0x1a3a4a, 0x0d1f28);
  scene.add(grid);
  var boxGeo = new THREE.BoxGeometry(20, 6, 20);
  var edges = new THREE.EdgesGeometry(boxGeo);
  scene.add(new THREE.LineSegments(edges, new THREE.LineBasicMaterial({ color: 0x1a4a5a, opacity: 0.3, transparent: true })).translateY(3));

  /* ---- signal field splats (20x20 floor) ---- */
  var GRID = 20, SIG = GRID * GRID;
  var sf = newSplatPoints(SIG);
  for (var iz = 0; iz < GRID; iz++) for (var ix = 0; ix < GRID; ix++) {
    var ii = iz * GRID + ix;
    sf.pos[ii * 3]     = (ix - GRID / 2) + 0.5;
    sf.pos[ii * 3 + 1] = 0.05;
    sf.pos[ii * 3 + 2] = (iz - GRID / 2) + 0.5;
    sf.size[ii] = 1.5;
    sf.opac[ii] = 0.15;
  }
  sf.geo.setAttribute('position', new THREE.BufferAttribute(sf.pos, 3));
  sf.geo.setAttribute('splatSize', new THREE.BufferAttribute(sf.size, 1));
  sf.geo.setAttribute('splatColor', new THREE.BufferAttribute(sf.col, 3));
  sf.geo.setAttribute('splatOpacity', new THREE.BufferAttribute(sf.opac, 1));
  scene.add(new THREE.Points(sf.geo, sf.mat));

  function newSplatPoints(count) {
    return {
      geo: new THREE.BufferGeometry(),
      pos: new Float32Array(count * 3),
      size: new Float32Array(count),
      col: new Float32Array(count * 3),
      opac: new Float32Array(count),
      mat: new THREE.ShaderMaterial({
        vertexShader: SPLAT_VERTEX,
        fragmentShader: SPLAT_FRAGMENT,
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending
      })
    };
  }

  function valueToColor(v, out, o) {
    v = Math.max(0, Math.min(1, v));
    var r, g, b;
    if (v < 0.5) { var t = v * 2; r = 0; g = t; b = 1 - t; }
    else { var t2 = (v - 0.5) * 2; r = t2; g = 1 - t2; b = 0; }
    out[o] = r; out[o + 1] = g; out[o + 2] = b;
  }

  /* ---- node markers (8-color palette, official) ---- */
  var NODE_COLORS = [0x00ccff, 0xff6600, 0x00ff88, 0xff00cc, 0xffcc00, 0x8800ff, 0x00ffcc, 0xff0044];
  var nodeObjs = {};
  var routerMarker = new THREE.Mesh(new THREE.SphereGeometry(0.3, 16, 16),
    new THREE.MeshBasicMaterial({ color: 0x00ff88, transparent: true, opacity: 0.8 }));
  routerMarker.position.set(0, 0.5, 0);
  scene.add(routerMarker);

  /* ---- body blobs (per person, breathing pulsation) ---- */
  var BLOB = 64;
  var MAX_P = 4;
  var blobPool = [];
  function makeBlob() {
    var b = newSplatPoints(BLOB);
    for (var i = 0; i < BLOB; i++) {
      var theta = Math.random() * Math.PI * 2;
      var phi = Math.acos(2 * Math.random() - 1);
      var r = Math.random() * 1.5;
      b.pos[i * 3]     = r * Math.sin(phi) * Math.cos(theta);
      b.pos[i * 3 + 1] = r * Math.cos(phi);
      b.pos[i * 3 + 2] = r * Math.sin(phi) * Math.sin(theta);
      b.size[i] = 2 + Math.random() * 3;
      b.col[i * 3] = 0.2; b.col[i * 3 + 1] = 0.8; b.col[i * 3 + 2] = 0.3;
      b.opac[i] = 0;
    }
    b.geo.setAttribute('position', new THREE.BufferAttribute(b.pos, 3));
    b.geo.setAttribute('splatSize', new THREE.BufferAttribute(b.size, 1));
    b.geo.setAttribute('splatColor', new THREE.BufferAttribute(b.col, 3));
    b.geo.setAttribute('splatOpacity', new THREE.BufferAttribute(b.opac, 1));
    var pts = new THREE.Points(b.geo, b.mat);
    var group = new THREE.Group();
    group.add(pts);
    /* posture label */
    var cv = document.createElement('canvas');
    cv.width = 256; cv.height = 64;
    var label = new THREE.CanvasTexture(cv);
    var sprite = new THREE.Sprite(new THREE.SpriteMaterial({ map: label, transparent: true, depthTest: false }));
    sprite.scale.set(1.6, 0.4, 1);
    sprite.position.y = 2.7;
    group.add(sprite);
    scene.add(group);
    return { group: group, pts: pts, b: b, cv: cv, label: label, sprite: sprite, active: false, lastPos: null, moveAcc: 0 };
  }

  function drawLabel(blob, text, conf) {
    var ctx = blob.cv.getContext('2d');
    ctx.clearRect(0, 0, blob.cv.width, blob.cv.height);
    ctx.font = 'bold 26px sans-serif';
    ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
    ctx.fillStyle = 'rgba(10,10,18,0.7)';
    var w = ctx.measureText(text).width + 24;
    ctx.fillRect(128 - w / 2, 4, w, 56);
    ctx.fillStyle = '#7fe7ff';
    ctx.fillText(text, 128, 30);
    if (conf !== undefined) { ctx.font = '15px sans-serif'; ctx.fillStyle = '#5a8ca8'; ctx.fillText((conf * 100).toFixed(0) + '%', 128, 50); }
    blob.label.needsUpdate = true;
  }

  /* ---- posture classification (COCO 17 px keypoints) ---- */
  function postureClassify(kp) {
    var avg = function (i1, i2, c) { var a = kp[i1], b = kp[i2]; if (!a || !b) return null; return (a[c] + b[c]) / 2; };
    var sY = avg(5, 6, 'y'), hY = avg(11, 12, 'y');
    var sX = avg(5, 6, 'x'), hX = avg(11, 12, 'x');
    var aY = avg(15, 16, 'y');
    var kY = avg(13, 14, 'y');
    if (sY == null || hY == null || aY == null) return '站立';
    var torsoPx = Math.abs(aY - sY);
    if (torsoPx < 1e-4) return '站立';
    var trunkTilt = Math.abs(sY - hY) / torsoPx;
    var hipHigh = (aY - hY) / torsoPx;
    var kneeBend = Math.abs(kY - hY) / torsoPx;
    if (trunkTilt > 0.55) return '卧';
    if (hipHigh < 0.45 && kneeBend < 0.25) return '坐';
    return '站立';
  }

  /* ---- frame update ---- */
  window.__rv3d = {
    update: function (m) {
      if (!m) return;
      var features = m.features || {};
      var classification = m.classification || {};
      var nodes = m.nodes || [];

      /* signal field splats */
      if (m.signal_field && m.signal_field.values && sf) {
        var vals = m.signal_field.values;
        var count = Math.min(vals.length, SIG);
        for (var i = 0; i < count; i++) {
          var v = vals[i];
          valueToColor(v, sf.col, i * 3);
          sf.size[i] = 1.0 + v * 4.0;
          sf.opac[i] = 0.1 + v * 0.6;
        }
        sf.geo.attributes.splatColor.needsUpdate = true;
        sf.geo.attributes.splatSize.needsUpdate = true;
        sf.geo.attributes.splatOpacity.needsUpdate = true;
      }

      /* node markers */
      var seen = {};
      for (var ni = 0; ni < nodes.length; ni++) {
        var n = nodes[ni];
        var id = n.node_id !== undefined ? n.node_id : ni;
        seen[id] = true;
        var obj = nodeObjs[id];
        if (!obj) {
          obj = new THREE.Mesh(new THREE.SphereGeometry(0.25, 16, 16),
            new THREE.MeshBasicMaterial({ color: NODE_COLORS[id % NODE_COLORS.length], transparent: true, opacity: 0.8 }));
          nodeObjs[id] = obj;
          scene.add(obj);
        }
        var np = n.position || [0, 0, 0];
        obj.position.set(np[0], 0.5, np[2]);
      }
      for (var oldId in nodeObjs) if (!seen[oldId]) { scene.remove(nodeObjs[oldId]); delete nodeObjs[oldId]; }

      /* body blobs with breathing pulsation */
      var used = 0;
      var breathPulse = 1.0 + Math.sin(Date.now() * 0.004) * Math.min((features.breathing_band_power || 0) * 3, 0.4);
      var motionLvl = classification.motion_level || 'absent';
      var presence = classification.presence || false;
      var confidence = classification.confidence || 0;

      if (m.persons) {
        for (var pi = 0; pi < m.persons.length && pi < MAX_P; pi++) {
          var person = m.persons[pi];
          var blob = blobPool[used];
          if (!blob) { blob = makeBlob(); blobPool.push(blob); }
          blob.active = true;
          blob.group.visible = true;
          /* room position */
          var pp = person.position || [0, 0, 0];
          blob.group.position.set(pp[0] || 0, 0, pp[2] || 0);
          if (blob.lastPos) {
            var d = Math.hypot((pp[0] || 0) - blob.lastPos.x, (pp[2] || 0) - blob.lastPos.z);
            blob.moveAcc = blob.moveAcc * 0.85 + d * 0.15;
          }
          blob.lastPos = { x: pp[0] || 0, z: pp[2] || 0 };
          var kp = person.keypoints || [];
          var posture = (kp.length >= 17) ? postureClassify(kp) : '站立';
          if (blob.moveAcc > 0.06) posture = '移动';
          var pconf = person.confidence || 0;
          drawLabel(blob, posture, pconf > 0 ? pconf : undefined);

          var b = blob.b;
          for (var bi = 0; bi < b.opac.length; bi++) {
            if (presence) {
              b.opac[bi] = confidence * 0.4;
              if (motionLvl === 'active') {
                b.col[bi * 3] = 1.0; b.col[bi * 3 + 1] = 0.2; b.col[bi * 3 + 2] = 0.1;
              } else {
                b.col[bi * 3] = 0.1; b.col[bi * 3 + 1] = 0.8; b.col[bi * 3 + 2] = 0.4;
              }
              b.size[bi] = (2 + Math.random() * 2) * breathPulse;
            } else {
              b.opac[bi] = 0.0;
            }
          }
          b.geo.attributes.splatOpacity.needsUpdate = true;
          b.geo.attributes.splatColor.needsUpdate = true;
          b.geo.attributes.splatSize.needsUpdate = true;
          used++;
        }
      }
      for (var pi2 = used; pi2 < blobPool.length; pi2++) {
        if (blobPool[pi2].active) { blobPool[pi2].active = false; blobPool[pi2].group.visible = false; }
      }
      /* remember breathing for idle pulses even with no person frame */
      window.__rv3d._breath = breathPulse;
      window.__rv3d._presence = presence;
    }
  };

  function animate() {
    requestAnimationFrame(animate);
    try {
    controls.update();
    /* router glow pulse (official) */
    routerMarker.material.opacity = 0.6 + 0.3 * Math.sin(Date.now() * 0.003);
    /* keep breathing pulse alive between person frames */
    var bp = window.__rv3d._breath;
    if (bp !== undefined && blobPool.length && window.__rv3d._presence) {
      for (var k = 0; k < blobPool.length; k++) {
        var b = blobPool[k];
        if (!b || !b.active || !b.b || !b.b.geo) continue;
        var sz = b.b.geo.attributes.splatSize.array;
        for (var i = 0; i < sz.length; i++) sz[i] = (2 + Math.random() * 2) * bp;
        b.b.geo.attributes.splatSize.needsUpdate = true;
      }
    }
    renderer.render(scene, camera);
    } catch (err) { diag('3D 渲染错误: ' + (err && err.message ? err.message : err), '#ff6b6b'); }
  }
  animate();
  diag('3D v3 已启动（高斯光斑）');

  window.addEventListener('resize', function () {
    var nw = container.clientWidth, nh = container.clientHeight;
    if (nw < 50 || nh < 50) return;
    camera.aspect = nw / nh; camera.updateProjectionMatrix();
    renderer.setSize(nw, nh);
  });
  } catch (err) { diag('3D 初始化错误: ' + (err && err.message ? err.message : err), '#ff6b6b'); }
})();

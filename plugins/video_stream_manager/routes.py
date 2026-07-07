"""VSM 插件 — API 代理路由 + 管理页面"""

from flask import Blueprint, request, jsonify, render_template, redirect, url_for
import requests
import json
import logging

from .models import (
    get_config, set_config, delete_config,
    VSM_API_URL_KEY, VSM_API_KEY_KEY,
    HLS_BASE_URL_KEY, WHEP_BASE_URL_KEY,
    upsert_device, get_cached_devices, get_cached_device,
    delete_cached_device, update_proxy_status, list_groups,
)
from plugins.base import BasePlugin

logger = logging.getLogger("vsm-plugin")
bp = Blueprint("vsm", __name__, url_prefix="/plugin/vsm",
               template_folder="templates", static_folder="static")


# ── 工具函数 ──────────────────────────────────────────────

def _vsm_api(method, path, **kwargs):
    """代理请求到 VSM 后端"""
    api_url = get_config(VSM_API_URL_KEY)
    if not api_url:
        return {"error": "VSM 未配置"}
    api_key = get_config(VSM_API_KEY_KEY)
    url = f"{api_url.rstrip('/')}/{path.lstrip('/')}"
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
    try:
        r = requests.request(method, url, headers=headers,
                             timeout=kwargs.pop("timeout", 10), **kwargs)
        return r.json() if r.text else {"status": r.status_code}
    except requests.ConnectionError:
        return {"error": "VSM 无法连接"}
    except Exception as e:
        return {"error": str(e)}


def _hls_url(device_id):
    """获取设备 HLS 播放 URL"""
    hls_base = get_config(HLS_BASE_URL_KEY)
    if not hls_base:
        api_url = get_config(VSM_API_URL_KEY)
        if api_url:
            hls_base = api_url.replace(":8899", ":8888")
    if not hls_base:
        return None
    return f"{hls_base.rstrip('/')}/live/{device_id}/index.m3u8"


def _whep_url(device_id):
    """获取设备 WebRTC WHEP URL"""
    whep_base = get_config(WHEP_BASE_URL_KEY)
    if not whep_base:
        api_url = get_config(VSM_API_URL_KEY)
        if api_url:
            whep_base = api_url.replace(":8899", ":8889")
    if not whep_base:
        return None
    return f"{whep_base.rstrip('/')}/live/{device_id}/whep"


# ── 管理页面 ──────────────────────────────────────────────

@bp.route("/")
def index():
    """VSM 管理后台首页"""
    api_url = get_config(VSM_API_URL_KEY)
    devices = get_cached_devices()
    groups = list_groups()
    return render_template("vsm_index.html",
                           api_url=api_url,
                           devices=devices,
                           groups=groups)


@bp.route("/settings")
def settings_page():
    """VSM 配置页面"""
    return render_template("vsm_settings.html",
                           api_url=get_config(VSM_API_URL_KEY) or "",
                           hls_url=get_config(HLS_BASE_URL_KEY) or "",
                           whep_url=get_config(WHEP_BASE_URL_KEY) or "",
                           api_key=get_config(VSM_API_KEY_KEY) or "")


@bp.route("/live/<device_id>")
def live_page(device_id):
    """单个设备直播页面"""
    dev = get_cached_device(device_id)
    if not dev:
        # 尝试从 VSM 获取
        resp = _vsm_api("GET", f"/api/devices/{device_id}")
        if resp and not resp.get("error"):
            upsert_device(resp)
            dev = resp
    if not dev:
        return "设备不存在", 404
    return render_template("vsm_live.html",
                           device=dev,
                           hls_url=_hls_url(device_id),
                           whep_url=_whep_url(device_id))


# ── 配置 API ──────────────────────────────────────────────

@bp.route("/api/config", methods=["GET"])
def get_config_api():
    """获取所有配置项"""
    return jsonify({
        VSM_API_URL_KEY: get_config(VSM_API_URL_KEY) or "",
        VSM_API_KEY_KEY: get_config(VSM_API_KEY_KEY) or "",
        HLS_BASE_URL_KEY: get_config(HLS_BASE_URL_KEY) or "",
        WHEP_BASE_URL_KEY: get_config(WHEP_BASE_URL_KEY) or "",
    })


@bp.route("/api/config", methods=["POST"])
def set_config_api():
    """保存配置项"""
    data = request.get_json() or {}
    for key in [VSM_API_URL_KEY, HLS_BASE_URL_KEY, WHEP_BASE_URL_KEY]:
        if key in data:
            set_config(key, data[key].strip())
    return jsonify({"status": "ok"})


# ── 设备管理 API ──────────────────────────────────────────

@bp.route("/api/devices")
def list_devices():
    """获取设备列表（优先从 VSM 拉实时数据，回退缓存）"""
    resp = _vsm_api("GET", "/api/devices", timeout=5)
    if isinstance(resp, list) and not (resp and isinstance(resp[0], dict) and resp[0].get("error")):
        for d in resp:
            upsert_device(d)
        return jsonify(resp)
    # VSM 不可达，从缓存返回
    devices = get_cached_devices()
    return jsonify(devices)


@bp.route("/api/devices/<device_id>")
def get_device(device_id):
    """获取单个设备"""
    resp = _vsm_api("GET", f"/api/devices/{device_id}", timeout=5)
    if resp and not resp.get("error"):
        upsert_device(resp)
        return jsonify(resp)
    dev = get_cached_device(device_id)
    if dev:
        return jsonify(dev)
    return jsonify({"error": "not found"}), 404


@bp.route("/api/devices/sync")
def sync_devices():
    """从 VSM 同步设备到本地数据库"""
    resp = _vsm_api("GET", "/api/devices", timeout=5)
    if resp and isinstance(resp, list):
        for d in resp:
            upsert_device(d)
        return jsonify({"status": "ok", "count": len(resp)})
    return jsonify(resp if resp else {"error": "sync failed"}), 502


@bp.route("/api/devices/<device_id>/delete", methods=["POST"])
def delete_device(device_id):
    """删除设备"""
    _vsm_api("DELETE", f"/api/devices/{device_id}")
    delete_cached_device(device_id)
    return jsonify({"status": "deleted"})


# ── 代理控制 API ─────────────────────────────────────────

@bp.route("/api/proxy/<device_id>/start", methods=["POST"])
def start_proxy(device_id):
    """启动设备代理"""
    resp = _vsm_api("POST", f"/api/proxy/start/{device_id}")
    if resp and not resp.get("error"):
        update_proxy_status(device_id, True)
        hls = _hls_url(device_id)
        return jsonify({"status": "started", "device_id": device_id, "hls_url": hls})
    return jsonify(resp or {"error": "start failed"}), 502


@bp.route("/api/proxy/<device_id>/stop", methods=["POST"])
def stop_proxy(device_id):
    """停止设备代理"""
    resp = _vsm_api("POST", f"/api/proxy/stop/{device_id}")
    update_proxy_status(device_id, False)
    return jsonify({"status": "stopped", "device_id": device_id})


# ── 远程控制 API（转发到手机 HTTP API） ─────────────────

@bp.route("/api/phone/<device_id>/<path:command>", methods=["POST"])
def phone_command(device_id, command):
    """通过 VSM 转发手机摄像头控制指令"""
    resp = _vsm_api("POST", f"/api/phone/{device_id}/{command}",
                    data=request.get_data(),
                    headers={"Content-Type": request.content_type or "application/json"})
    return jsonify(resp if resp else {"error": "command failed"})


# ── 分组 API ──────────────────────────────────────────────

@bp.route("/api/groups")
def list_groups_api():
    """获取设备分组列表"""
    return jsonify(list_groups())

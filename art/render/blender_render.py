"""Blender-рендерер пайплайна ART-2: модели из models.json -> toon + обводка (inverted hull) -> изометрия 2:1
-> кадры PNG по одному на файл (упаковку в атласы и манифест делает art/tools/build_art.py).
Основан на art/spike/blender/render_spike.py (ART-1), 8 направлений для юнита, 1 кадр для здания/пропа.

Не запускать руками: вызывает build_art.py.  Вручную:
  blender -b -P art/render/blender_render.py -- --models art/src/units/models.json --palette art/src/palette.json --out DIR --ids u_rifleman
Параметры после `--`: --models FILE  --palette FILE  --out DIR  --ids a,b (пусто = все)  --ss N (по умолчанию 4)
Результат: DIR/<id>/frames/<имя>.png и DIR/<id>/meta.json ({id, kind, dirs, anchor, frame, anims{имя:{fps,frames,loop}}, frames[имена по порядку]}).
"""
import json
import math
import os
import struct
import sys
import time
import zlib

import warnings

import bpy
import mathutils
import numpy as np

warnings.filterwarnings("ignore", category=DeprecationWarning)  # Material.use_nodes: убирается в Blender 6.0, до этого нужен для 4.x
argv = sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else []


def opt(name, default=None):
    return argv[argv.index(name) + 1] if name in argv else default


OUT = os.path.abspath(opt("--out"))
SS = int(opt("--ss", 4))
ONLY = set(opt("--ids", "").split(",")) - {""}
DIRS = 8
OUTLINE_PX = {"unit": 1.0, "building": 1.6}
PX_PER_UNIT = 32 * math.sqrt(2)          # px на единицу вдоль экранных осей: ребро тайла 64x32
ELEV = math.radians(30)                   # 2:1 изометрия
pal = json.load(open(opt("--palette"), encoding="utf-8"))
models = json.load(open(opt("--models"), encoding="utf-8"))["models"]


def srgb_to_lin(c):
    return c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4


def hexlin(h):
    return [srgb_to_lin(int(h[i:i + 2], 16) / 255) for i in (1, 3, 5)] + [1.0]


# ---------- сцена ----------
def setup_scene(frame_w, frame_h):
    bpy.ops.wm.read_factory_settings(use_empty=True)
    _mats.clear()
    sc = bpy.context.scene
    engines = [e.identifier for e in sc.render.bl_rna.properties["engine"].enum_items]
    sc.render.engine = "BLENDER_EEVEE" if "BLENDER_EEVEE" in engines else "BLENDER_EEVEE_NEXT"
    sc.render.film_transparent = True
    sc.render.resolution_x, sc.render.resolution_y, sc.render.resolution_percentage = frame_w * SS, frame_h * SS, 100
    sc.render.image_settings.file_format = "PNG"
    sc.render.image_settings.color_mode = "RGBA"
    sc.render.image_settings.color_depth = "8"
    sc.view_settings.view_transform = "Standard"
    sc.view_settings.look = "None"
    try:
        sc.eevee.taa_render_samples = 8
    except Exception:
        pass
    sc.world = bpy.data.worlds.new("w")
    # свет: sun вдоль -dir (dir = от поверхности к свету), без теней — cel-полосы считаются только по n·L
    d = mathutils.Vector(pal["light"]["dir"]).normalized()
    ld = bpy.data.lights.new("sun", "SUN")
    ld.energy = 1.0
    try:
        ld.use_shadow = False
    except Exception:
        pass
    lo = bpy.data.objects.new("sun", ld)
    lo.rotation_euler = (-d).to_track_quat("-Z", "Y").to_euler()
    sc.collection.objects.link(lo)
    return sc


def setup_camera(sc, frame_w, frame_h, anchor):
    """Ортокамера 2:1: анкер (px кадра) <-> мировая точка (0,0,0)."""
    v = mathutils.Vector((math.cos(ELEV) * math.sqrt(.5), math.cos(ELEV) * math.sqrt(.5), math.sin(ELEV)))
    # Камера в (+x,+y) смотрит «зеркально» относительно сетки клиента (экранное право = (x-y)): её право = (-x,+y).
    # Рендерим так и разворачиваем кадр по горизонтали (render_frame), поэтому анкер считаем в зеркальном кадре.
    right = mathutils.Vector((-math.sqrt(.5), math.sqrt(.5), 0))
    up = v.cross(right).normalized()
    up = -up if up.z < 0 else up
    ax, ay = frame_w - anchor[0], anchor[1]
    p = right * ((frame_w / 2 - ax) / PX_PER_UNIT) - up * ((frame_h / 2 - ay) / PX_PER_UNIT)
    cam = bpy.data.cameras.new("cam")
    cam.type = "ORTHO"
    cam.ortho_scale = max(frame_w, frame_h) / PX_PER_UNIT
    cam.clip_start, cam.clip_end = 0.1, 200
    co = bpy.data.objects.new("cam", cam)
    co.location = p + v * 50
    co.rotation_euler = (-v).to_track_quat("-Z", "Y").to_euler()
    sc.collection.objects.link(co)
    sc.camera = co


# ---------- материалы ----------
_mats = {}


def toon_mat(key):
    if key in _mats:
        return _mats[key]
    m = bpy.data.materials.new(key)
    try:
        m.use_nodes = True
    except Exception:
        pass
    nt = m.node_tree
    nt.nodes.clear()
    diff = nt.nodes.new("ShaderNodeBsdfDiffuse")
    diff.inputs["Color"].default_value = (1, 1, 1, 1)
    s2r = nt.nodes.new("ShaderNodeShaderToRGB")
    ramp = nt.nodes.new("ShaderNodeValToRGB")
    ramp.color_ramp.interpolation = "CONSTANT"
    bands = sorted(pal["light"]["bands"], key=lambda b: b["min"])  # по возрастанию порога
    cr = ramp.color_ramp
    while len(cr.elements) < len(bands):
        cr.elements.new(0.5)
    for el, b in zip(cr.elements, bands):
        el.position = max(0.0, b["min"])
        el.color = [srgb_to_lin(x) for x in b["mul"]] + [1.0]
    mix = nt.nodes.new("ShaderNodeMix")
    mix.data_type = "RGBA"
    mix.blend_type = "MULTIPLY"
    mix.inputs["Factor"].default_value = 1.0
    mix.inputs["B"].default_value = hexlin(pal[key])
    emis = nt.nodes.new("ShaderNodeEmission")
    out = nt.nodes.new("ShaderNodeOutputMaterial")
    L = nt.links.new
    L(diff.outputs["BSDF"], s2r.inputs["Shader"])
    L(s2r.outputs["Color"], ramp.inputs["Fac"])
    L(ramp.outputs["Color"], mix.inputs["A"])
    L(mix.outputs["Result"], emis.inputs["Color"])
    L(emis.outputs["Emission"], out.inputs["Surface"])
    _mats[key] = m
    return m


def outline_mat():
    if "_outline" in _mats:
        return _mats["_outline"]
    m = bpy.data.materials.new("outline")
    try:
        m.use_nodes = True
    except Exception:
        pass
    nt = m.node_tree
    nt.nodes.clear()
    em = nt.nodes.new("ShaderNodeEmission")
    em.inputs["Color"].default_value = hexlin(pal["outline"])
    out = nt.nodes.new("ShaderNodeOutputMaterial")
    nt.links.new(em.outputs["Emission"], out.inputs["Surface"])
    m.use_backface_culling = True   # inverted hull: видны только «внутренние» грани оболочки
    _mats["_outline"] = m
    return m


# ---------- меши ----------
SEG = 12


def mesh_data(shape, s):
    sx, sy, sz = s
    x, y, z = sx / 2, sy / 2, sz / 2
    if shape == "box":
        v = [(-x, -y, -z), (x, -y, -z), (x, y, -z), (-x, y, -z), (-x, -y, z), (x, -y, z), (x, y, z), (-x, y, z)]
        f = [(0, 3, 2, 1), (4, 5, 6, 7), (0, 1, 5, 4), (2, 3, 7, 6), (1, 2, 6, 5), (3, 0, 4, 7)]
        return v, f, False
    if shape in ("cyl", "cone"):
        v = [(math.cos(i / SEG * 2 * math.pi) * x, math.sin(i / SEG * 2 * math.pi) * y, -z) for i in range(SEG)]
        if shape == "cyl":
            v += [(math.cos(i / SEG * 2 * math.pi) * x, math.sin(i / SEG * 2 * math.pi) * y, z) for i in range(SEG)]
            f = [tuple(reversed(range(SEG))) if False else tuple(range(SEG - 1, -1, -1)), tuple(range(SEG, 2 * SEG))]
            f += [(i, (i + 1) % SEG, SEG + (i + 1) % SEG, SEG + i) for i in range(SEG)]
        else:
            v.append((0, 0, z))
            f = [tuple(range(SEG - 1, -1, -1))] + [(i, (i + 1) % SEG, SEG) for i in range(SEG)]
        return v, f, True
    if shape == "sphere":
        R, S = 8, 12
        v = [(0, 0, z)]
        for r in range(1, R):
            th = r / R * math.pi
            v += [(math.sin(th) * math.cos(i / S * 2 * math.pi) * x, math.sin(th) * math.sin(i / S * 2 * math.pi) * y, math.cos(th) * z) for i in range(S)]
        v.append((0, 0, -z))
        f = [(0, 1 + (i + 1) % S, 1 + i) for i in range(S)]
        for r in range(R - 2):
            a, b = 1 + r * S, 1 + (r + 1) * S
            f += [(a + i, a + (i + 1) % S, b + (i + 1) % S, b + i) for i in range(S)]
        last, c = 1 + (R - 2) * S, len(v) - 1
        f += [(last + i, last + (i + 1) % S, c) for i in range(S)]
        return v, f, True
    raise ValueError(shape)


def make_part(pt, kind, parent, pivot):
    v, f, smooth = mesh_data(pt["shape"], pt["s"])
    me = bpy.data.meshes.new(pt["n"])
    me.from_pydata(v, [], f)
    me.update()
    me.polygons.foreach_set("use_smooth", [smooth] * len(me.polygons))
    ob = bpy.data.objects.new(pt["n"], me)
    ob.data.materials.append(toon_mat(pt["col"]))
    ob.data.materials.append(outline_mat())
    c = pt["c"]
    ob.location = (c[0] - pivot[0], c[1] - pivot[1], c[2] - pivot[2])
    ob.rotation_euler = [math.radians(a) for a in pt.get("rot", [0, 0, 0])]
    ob.parent = parent
    bpy.context.scene.collection.objects.link(ob)
    bev = ob.modifiers.new("bevel", "BEVEL")
    bev.width = 0.012 if kind == "unit" else 0.025
    bev.segments = 1
    bev.limit_method = "ANGLE"
    sol = ob.modifiers.new("outline", "SOLIDIFY")
    sol.thickness = OUTLINE_PX[kind] / PX_PER_UNIT
    sol.offset = 1.0
    sol.use_flip_normals = True
    sol.material_offset = 1
    return ob


def build_model(model):
    """-> (yaw_empty, {group: empty}). Иерархия: fall -> yaw -> groups -> parts."""
    kind = model["kind"]
    fall = bpy.data.objects.new("fall", None)
    yaw = bpy.data.objects.new("yaw", None)
    fall.rotation_mode = "AXIS_ANGLE"
    yaw.parent = fall
    for o in (fall, yaw):
        bpy.context.scene.collection.objects.link(o)
    groups = {}
    for name, g in model.get("groups", {}).items():
        e = bpy.data.objects.new(name, None)
        bpy.context.scene.collection.objects.link(e)
        groups[name] = e
    for name, g in model.get("groups", {}).items():
        e = groups[name]
        par = groups[g["parent"]] if g.get("parent") else yaw
        e.parent = par
        pp = model["groups"][g["parent"]]["pivot"] if g.get("parent") else [0, 0, 0]
        e.location = [a - b for a, b in zip(g["pivot"], pp)]
    for pt in model["parts"]:
        gname = pt.get("g")
        parent = groups[gname] if gname else yaw
        make_part(pt, kind, parent, model["groups"][gname]["pivot"] if gname else [0, 0, 0])
    return fall, yaw, groups


def apply_pose(model, groups, pose):
    for name, e in groups.items():
        p = pose.get(name, {})
        piv = model["groups"][name]["pivot"]
        par = model["groups"][name].get("parent")
        pp = model["groups"][par]["pivot"] if par else [0, 0, 0]
        e.location = [piv[i] - pp[i] + p.get(("dx", "dy", "dz")[i], 0) for i in range(3)]
        e.rotation_euler = [math.radians(p.get(k, 0)) for k in ("rx", "ry", "rz")]


def frame_pose(model, anim, i):
    out = json.loads(json.dumps(model.get("rest", {})))
    for g, v in anim["frames"][i].items():
        if not g.startswith("_"):
            out.setdefault(g, {}).update(v)
    return out


# ---------- рендер кадра ----------
def render_frame(path):
    bpy.context.scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    img = bpy.data.images.load(path)
    img.colorspace_settings.name = "Non-Color"
    w, h = img.size
    a = np.array(img.pixels[:], dtype=np.float32).reshape(h, w, 4)[::-1, ::-1]
    bpy.data.images.remove(img)
    return a


def downsample(a):
    h, w, _ = a.shape
    a = a.reshape(h // SS, SS, w // SS, SS, 4)
    al = a[..., 3:4]
    pm = (a[..., :3] * al).sum(axis=(1, 3))
    asum = al.sum(axis=(1, 3))
    rgb = np.where(asum > 0, pm / np.maximum(asum, 1e-8), 0)
    alpha = asum / (SS * SS)
    return np.clip(np.concatenate([rgb, alpha], axis=2) * 255 + 0.5, 0, 255).astype(np.uint8)


def render_model(model, tmp):
    kind = model["kind"]
    fw, fh = model["frame"]
    sc = setup_scene(fw, fh)
    setup_camera(sc, fw, fh, model["anchor"])
    fall, yaw, groups = build_model(model)
    out = []
    t0 = time.time()
    if kind == "building":
        out.append((f"{model['id']}_0", downsample(render_frame(os.path.join(tmp, "f.png")))))
    else:
        # тень-пятно — отдельный диск под юнитом
        sh = bpy.data.objects.new("shadow", bpy.data.meshes.new("shadow"))
        v = [(math.cos(i / 16 * 2 * math.pi) * 0.3, math.sin(i / 16 * 2 * math.pi) * 0.3, 0.001) for i in range(16)]
        sh.data.from_pydata(v, [], [tuple(range(16))])
        sm = bpy.data.materials.new("shadow")
        sm.use_nodes = True
        nt = sm.node_tree
        nt.nodes.clear()
        tr = nt.nodes.new("ShaderNodeBsdfTransparent")
        em = nt.nodes.new("ShaderNodeEmission")
        em.inputs["Color"].default_value = (0.01, 0.015, 0.01, 1)
        mx = nt.nodes.new("ShaderNodeMixShader")
        mx.inputs["Fac"].default_value = 0.28
        o = nt.nodes.new("ShaderNodeOutputMaterial")
        nt.links.new(tr.outputs[0], mx.inputs[1])
        nt.links.new(em.outputs[0], mx.inputs[2])
        nt.links.new(mx.outputs[0], o.inputs[0])
        sh.data.materials.append(sm)
        bpy.context.scene.collection.objects.link(sh)
        for an, anim in model["anims"].items():
            for d in range(DIRS):
                for n in range(len(anim["frames"])):
                    apply_pose(model, groups, frame_pose(model, anim, n))
                    yaw.rotation_euler = (0, 0, math.radians(45 + 360 / DIRS * d))
                    fall.rotation_axis_angle = (math.radians(anim["frames"][n].get("_fall", 0)), math.sqrt(.5), -math.sqrt(.5), 0)
                    arr = render_frame(os.path.join(tmp, "f.png"))
                    out.append((f"{model['id']}_{an}_d{d}_{n}", downsample(arr)))
    return out, time.time() - t0


# ---------- PNG / атлас ----------
def write_png(path, arr):
    h, w, _ = arr.shape
    raw = b"".join(b"\x00" + arr[y].tobytes() for y in range(h))

    def chunk(t, d):
        c = struct.pack(">I", len(d)) + t + d
        return c + struct.pack(">I", zlib.crc32(t + d) & 0xFFFFFFFF)

    with open(path, "wb") as f:
        f.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def main():
    t_all = time.time()
    ms = [m for m in models if not ONLY or m["id"] in ONLY]
    if ONLY and len(ms) != len(ONLY):
        sys.exit("нет моделей: " + ", ".join(sorted(ONLY - {m["id"] for m in ms})))
    for m in ms:
        d = os.path.join(OUT, m["id"])
        fd = os.path.join(d, "frames")
        os.makedirs(fd, exist_ok=True)
        tmp = os.path.join(d, "_raw")
        os.makedirs(tmp, exist_ok=True)
        fr, dt = render_model(m, tmp)
        for n, a in fr:
            write_png(os.path.join(fd, n + ".png"), a)
        for f in os.listdir(tmp):
            os.remove(os.path.join(tmp, f))
        os.rmdir(tmp)
        anims = {k: {"fps": a["fps"], "frames": len(a["frames"]), "loop": a.get("loop", True)} for k, a in m.get("anims", {}).items()}
        meta = {"id": m["id"], "kind": m["kind"], "renderer": "blender", "dirs": DIRS if m["kind"] == "unit" else 1,
                "anchor": m["anchor"], "frame": m["frame"], "anims": anims, "frames": [n for n, _ in fr]}
        json.dump(meta, open(os.path.join(d, "meta.json"), "w", encoding="utf-8", newline="\n"), indent=1)
        print(f"{m['id']}: {len(fr)} frames, {dt:.1f} s", flush=True)
    print(f"done in {time.time() - t_all:.1f} s -> {OUT} (blender {bpy.app.version_string})")


main()

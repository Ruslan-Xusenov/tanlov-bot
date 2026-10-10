import os
import sys
import glob
import json
import random
import asyncio
from datetime import datetime
from telethon import TelegramClient, events, Button
from telethon.errors import (
    FloodWaitError,
    PeerFloodError,
    UserPrivacyRestrictedError,
    ChatAdminRequiredError,
    SessionPasswordNeededError,
)

# my.telegram.org ma'lumotlari
API_ID = 34834500
API_HASH = "253bacb0cb1d219707d69310d1beb404"

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
SESSIONS_DIR = os.path.join(BASE_DIR, "sessions")
CONFIG_FILE = os.path.join(BASE_DIR, "config.json")
SENT_LOG_FILE = os.path.join(BASE_DIR, "sent_users.txt")
BOT_SESSION_PATH = os.path.join(BASE_DIR, "bot_session")

os.makedirs(SESSIONS_DIR, exist_ok=True)

# ─────────────────────────────────────────────────────────────
# Sozlamalar va Fayllar
# ─────────────────────────────────────────────────────────────

def load_config():
    default_cfg = {
        "bot_token": "8972496719:AAGihCYed-kEtAcSi1gf0_6wdmCrUsYSXs4",
        "admin_id": 8438648642,
        "channel": "@shshdhdhdwiw",
        "min_delay": 5,
        "max_delay": 12,
        "batch_size": 15,
        "pause_duration": 60,
        "mode": "forward",
        "is_running": True,
    }
    if os.path.exists(CONFIG_FILE):
        try:
            with open(CONFIG_FILE, "r", encoding="utf-8") as f:
                data = json.load(f)
                default_cfg.update(data)
                return default_cfg
        except Exception:
            return default_cfg
    return default_cfg


def save_config(cfg):
    with open(CONFIG_FILE, "w", encoding="utf-8") as f:
        json.dump(cfg, f, ensure_ascii=False, indent=2)


def get_sent_users_count():
    if not os.path.exists(SENT_LOG_FILE):
        return 0
    with open(SENT_LOG_FILE, "r", encoding="utf-8") as f:
        return len([line for line in f if line.strip()])


def record_sent_user(user_id):
    with open(SENT_LOG_FILE, "a", encoding="utf-8") as f:
        f.write(f"{user_id}\n")


# ─────────────────────────────────────────────────────────────
# Akkauntlar Havzasi (Multi-Account Pool)
# ─────────────────────────────────────────────────────────────

class AccountPool:
    def __init__(self):
        self.accounts = []
        self.current_idx = 0

    async def load_all(self):
        for acc in self.accounts:
            try:
                await acc["client"].disconnect()
            except Exception:
                pass
        self.accounts.clear()

        session_files = sorted(glob.glob(os.path.join(SESSIONS_DIR, "*.session")))
        for s_path in session_files:
            s_name = os.path.splitext(s_path)[0]
            try:
                client = TelegramClient(s_name, API_ID, API_HASH)
                await client.connect()
                if not await client.is_user_authorized():
                    await client.disconnect()
                    continue

                me = await client.get_me()
                name = me.first_name or f"ID: {me.id}"
                self.accounts.append({
                    "client": client,
                    "me": me,
                    "name": name,
                    "username": f"@{me.username}" if me.username else "yo'q",
                    "id": me.id,
                    "session_path": s_path,
                    "cooldown_until": 0,
                })
            except Exception as e:
                print(f"[pool] Sessiyani yuklashda xatolik ({os.path.basename(s_path)}): {e}")

        return self.accounts

    def get_available_account(self):
        if not self.accounts:
            return None
        now = asyncio.get_event_loop().time()
        for _ in range(len(self.accounts)):
            acc = self.accounts[self.current_idx]
            self.current_idx = (self.current_idx + 1) % len(self.accounts)
            if acc["cooldown_until"] <= now:
                return acc
        return min(self.accounts, key=lambda a: a["cooldown_until"])

    def set_cooldown(self, acc, seconds):
        now = asyncio.get_event_loop().time()
        acc["cooldown_until"] = now + seconds


# ─────────────────────────────────────────────────────────────
# Xabarlarni jo'natish yadrosi
# ─────────────────────────────────────────────────────────────

async def get_channel_subscribers(client, channel):
    subscribers = []
    try:
        async for user in client.iter_participants(channel):
            if not user.bot and not user.is_self and not user.deleted:
                subscribers.append(user)
    except Exception as e:
        print(f"[subscribers] Xatolik: {e}")
        return None
    return subscribers


async def send_single_user(client, user_id, messages, mode, channel):
    sent_ok = False
    if mode == "forward":
        try:
            msg_ids = [m.id for m in messages]
            await client.forward_messages(user_id, msg_ids, from_peer=channel)
            sent_ok = True
        except Exception as fe:
            err_str = str(fe).lower()
            if "protected chat" in err_str or "forward" in err_str:
                sent_ok = False
            else:
                raise fe

    if not sent_ok:
        if len(messages) == 1:
            msg = messages[0]
            if msg.media:
                try:
                    await client.send_file(
                        user_id,
                        file=msg.media,
                        caption=msg.text,
                        formatting_entities=msg.entities,
                    )
                except Exception:
                    temp_file = await client.download_media(msg)
                    try:
                        await client.send_file(
                            user_id,
                            file=temp_file,
                            caption=msg.text,
                            formatting_entities=msg.entities,
                        )
                    finally:
                        if temp_file and os.path.exists(temp_file):
                            os.remove(temp_file)
            else:
                await client.send_message(
                    user_id,
                    message=msg.text,
                    formatting_entities=msg.entities,
                )
        else:
            temp_paths = []
            try:
                for m in messages:
                    if m.media:
                        p = await client.download_media(m)
                        if p:
                            temp_paths.append(p)
                if temp_paths:
                    caption_text = next((m.text for m in messages if m.text), "")
                    caption_ent = next((m.entities for m in messages if m.entities), None)
                    await client.send_file(
                        user_id,
                        file=temp_paths,
                        caption=caption_text,
                        formatting_entities=caption_ent,
                    )
            finally:
                for p in temp_paths:
                    if p and os.path.exists(p):
                        os.remove(p)


async def distribute_messages_multi(bot_client, admin_id, pool, channel, messages, cfg):
    if not pool.accounts:
        await bot_client.send_message(admin_id, "❌ Hech qanday faol akkaunt yo'q! Avval akkaunt ulang.")
        return

    admin_acc = pool.accounts[0]
    subscribers = await get_channel_subscribers(admin_acc["client"], channel)
    if subscribers is None:
        await bot_client.send_message(admin_id, f"❌ Kanal a'zolarini olib bo'lmadi. <b>{admin_acc['name']}</b> bu kanalda Admin ekanligini tekshiring!", parse_mode="html")
        return

    total = len(subscribers)
    if total == 0:
        await bot_client.send_message(admin_id, "ℹ️ Kanaldan a'zolar topilmadi.")
        return

    min_delay = cfg.get("min_delay", 5)
    max_delay = cfg.get("max_delay", 12)
    batch_size = cfg.get("batch_size", 15)
    pause_duration = cfg.get("pause_duration", 60)
    mode = cfg.get("mode", "forward")

    start_notice = (
        f"📢 <b>Kanalda yangi post aniqlandi!</b>\n\n"
        f"👥 Jami obunachilar: <b>{total} ta</b>\n"
        f"⚡️ Faol akkauntlar: <b>{len(pool.accounts)} ta</b>\n"
        f"⏱ Kechikish: {min_delay}–{max_delay}s | ☕️ Tanaffus: Har {batch_size} tadan so'ng {pause_duration}s\n\n"
        f"🚀 <i>Tarqatish jarayoni boshlandi...</i>"
    )
    await bot_client.send_message(admin_id, start_notice, parse_mode="html")

    success = 0
    failed = 0
    batch_counter = 0

    for idx, user in enumerate(subscribers, start=1):
        acc = pool.get_available_account()
        if not acc:
            break

        now_time = asyncio.get_event_loop().time()
        if acc["cooldown_until"] > now_time:
            wait_sec = int(acc["cooldown_until"] - now_time) + 1
            await asyncio.sleep(wait_sec)

        client = acc["client"]
        try:
            await send_single_user(client, user.id, messages, mode, channel)
            success += 1
            batch_counter += 1
            record_sent_user(user.id)

            if batch_counter >= batch_size and idx < total:
                await bot_client.send_message(admin_id, f"☕️ <b>Xavfsizlik tanaffusi:</b> {batch_size} ta xabar ketdi. {pause_duration}s dam olinmoqda...", parse_mode="html")
                await asyncio.sleep(pause_duration)
                batch_counter = 0
            else:
                r_delay = round(random.uniform(min_delay, max_delay), 1)
                await asyncio.sleep(r_delay)

        except FloodWaitError as e:
            pool.set_cooldown(acc, e.seconds + 5)
        except PeerFloodError:
            pool.set_cooldown(acc, 7200)
            await bot_client.send_message(admin_id, f"⚠️ <b>{acc['name']}</b> akkauntiga Telegram SpamBlock qo'ydi. U vaqtincha dam olishga olindi.", parse_mode="html")
        except UserPrivacyRestrictedError:
            failed += 1
        except Exception as e:
            print(f"[send] Xatolik ({user.id}): {e}")
            failed += 1

    fin_notice = (
        f"✅ <b>Post tarqatish yakunlandi!</b>\n\n"
        f"📊 <b>Natija:</b>\n"
        f"• Muvaffaqiyatli: <b>{success} ta</b>\n"
        f"• Xatolik (lichkasi yopiqlar): <b>{failed} ta</b>"
    )
    await bot_client.send_message(admin_id, fin_notice, parse_mode="html")


# ─────────────────────────────────────────────────────────────
# Bot Interface & Admin State Management
# ─────────────────────────────────────────────────────────────

admin_states = {}
pending_logins = {}
media_group_buffers = {}
post_queue = asyncio.Queue()


def get_dashboard_text(cfg, pool):
    status_emoji = "🟢 FAOL (Ishlamoqda)" if cfg.get("is_running", True) else "🔴 TO'XTATILGAN"
    accounts_text = f"{len(pool.accounts)} ta"
    if pool.accounts:
        acc_names = [f"• {a['name']} ({a['username']})" for a in pool.accounts]
        accounts_text += "\n" + "\n".join(acc_names)

    return (
        f"🤖 <b>Multi-Account Rassilka Boshqaruv Paneli</b>\n\n"
        f"📊 <b>Holati:</b> {status_emoji}\n"
        f"📢 <b>Kanal:</b> <code>{cfg.get('channel')}</code>\n\n"
        f"👥 <b>Ulangan akkauntlar:</b>\n{accounts_text}\n\n"
        f"⏱ <b>Tasodifiy kechikish:</b> {cfg.get('min_delay')} – {cfg.get('max_delay')} soniya\n"
        f"☕️ <b>Tanaffus:</b> Har {cfg.get('batch_size')} tadan keyin {cfg.get('pause_duration')} soniya\n"
        f"🔄 <b>Yuborish turi:</b> {'Forward (Uzatish)' if cfg.get('mode') == 'forward' else 'Asl nusxa'}\n"
        f"📬 <b>Jami yuborilganlar:</b> {get_sent_users_count()} ta\n\n"
        f"<i>Quyidagi tugmalar orqali tizimni boshqaring:</i>"
    )


def get_dashboard_keyboard(cfg):
    run_btn = (
        Button.inline("⏹ Avto-tarqatishni to'xtatish", b"stop_auto")
        if cfg.get("is_running", True)
        else Button.inline("▶️ Avto-tarqatishni yoqish", b"start_auto")
    )
    return [
        [run_btn],
        [Button.inline("👥 Akkauntlar", b"menu_accounts"), Button.inline("➕ Akkaunt qo'shish", b"menu_add_acc")],
        [Button.inline("📢 Kanalni o'zgartirish", b"menu_channel"), Button.inline("⚙️ Vaqt va Tanaffus", b"menu_timing")],
        [Button.inline("✍️ Qo'lda xabar yuborish", b"menu_manual"), Button.inline("🔄 Yangilash", b"refresh_dash")],
    ]


async def queue_worker_job(bot_client, pool, cfg):
    while True:
        job = await post_queue.get()
        try:
            channel, messages = job
            admin_id = cfg.get("admin_id")
            await distribute_messages_multi(bot_client, admin_id, pool, channel, messages, cfg)
        except Exception as e:
            print(f"[queue] Xatolik: {e}")
        finally:
            post_queue.task_done()


async def setup_channel_listener(pool, cfg, bot_client):
    if not pool.accounts:
        return None

    admin_acc = pool.accounts[0]
    client = admin_acc["client"]
    channel_str = cfg.get("channel")

    try:
        channel = await client.get_entity(channel_str)
    except Exception as e:
        print(f"[listener] Kanalga ulanib bo'lmadi ({channel_str}): {e}")
        return None

    async def process_media_group(group_id):
        await asyncio.sleep(2.0)
        if group_id in media_group_buffers:
            msgs = media_group_buffers.pop(group_id, [])
            if msgs and cfg.get("is_running", True):
                await post_queue.put((channel, msgs))

    @client.on(events.NewMessage(chats=channel))
    async def channel_post_handler(event):
        if not cfg.get("is_running", True):
            return

        msg = event.message
        if not msg or msg.action:
            return

        if msg.grouped_id:
            gid = msg.grouped_id
            if gid not in media_group_buffers:
                media_group_buffers[gid] = []
                asyncio.create_task(process_media_group(gid))
            media_group_buffers[gid].append(msg)
        else:
            await post_queue.put((channel, [msg]))

    print(f"[listener] @{getattr(channel, 'username', channel_str)} kanali uchun avto-kuzatuv faol!")
    return channel


# ─────────────────────────────────────────────────────────────
# Asosiy dastur (Main)
# ─────────────────────────────────────────────────────────────

def is_admin(cfg, sender_id):
    cfg_admin = cfg.get("admin_id", 0)
    if not cfg_admin or cfg_admin == 0:
        cfg["admin_id"] = sender_id
        save_config(cfg)
        print(f"👑 Yangi admin biriktirildi: ID={sender_id}")
        return True
    if sender_id == cfg_admin:
        return True
    known_admins = [8438648642, 8721943455, 8462446411]
    return sender_id in known_admins


async def main():
    cfg = load_config()
    bot_token = cfg.get("bot_token")

    print("\n" + "=" * 60)
    print("🤖 Multi-Account Rassilka Admin Boti ishga tushmoqda...")
    print("=" * 60)

    # 1. Akkauntlarni yuklaymiz
    pool = AccountPool()
    await pool.load_all()
    print(f"✅ {len(pool.accounts)} ta akkaunt muvaffaqiyatli ulandi.")

    # 2. Telegram Botni ishga tushiramiz
    bot = TelegramClient(BOT_SESSION_PATH, API_ID, API_HASH)
    await bot.start(bot_token=bot_token)
    bot_me = await bot.get_me()
    print(f"🤖 Admin Bot ishga tushdi: @{bot_me.username}")

    # 3. Queue worker va Kanal tinglovchini yoqamiz
    asyncio.create_task(queue_worker_job(bot, pool, cfg))
    if pool.accounts:
        await setup_channel_listener(pool, cfg, bot)

    # ─────────────────── Callback Query Handlers ───────────────────

    @bot.on(events.CallbackQuery)
    async def on_callback(event):
        sender_id = event.sender_id
        if not is_admin(cfg, sender_id):
            await event.answer(f"❌ Ruxsat yo'q (ID: {sender_id})", alert=True)
            return

        data = event.data

        if data == b"refresh_dash":
            await event.edit(get_dashboard_text(cfg, pool), parse_mode="html", buttons=get_dashboard_keyboard(cfg))

        elif data == b"start_auto":
            cfg["is_running"] = True
            save_config(cfg)
            await event.answer("▶️ Avto-tarqatish yoqildi!", alert=False)
            await event.edit(get_dashboard_text(cfg, pool), parse_mode="html", buttons=get_dashboard_keyboard(cfg))

        elif data == b"stop_auto":
            cfg["is_running"] = False
            save_config(cfg)
            await event.answer("⏹ Avto-tarqatish to'xtatildi!", alert=False)
            await event.edit(get_dashboard_text(cfg, pool), parse_mode="html", buttons=get_dashboard_keyboard(cfg))

        elif data == b"menu_accounts":
            btns = []
            for acc in pool.accounts:
                btns.append([Button.inline(f"🗑 {acc['name']} ({acc['id']})", f"del_{acc['id']}".encode())])
            btns.append([Button.inline("➕ Yangi akkaunt qo'shish", b"menu_add_acc")])
            btns.append([Button.inline("🔙 Asosiy panel", b"refresh_dash")])
            await event.edit(
                f"👥 <b>Ulangan akkauntlar ro'yxati ({len(pool.accounts)} ta):</b>\n\n"
                f"Akkauntni o'chirish uchun ustiga bosing:",
                parse_mode="html",
                buttons=btns
            )

        elif data.startswith(b"del_"):
            target_id = int(data.decode().replace("del_", ""))
            target_acc = next((a for a in pool.accounts if a["id"] == target_id), None)
            if target_acc:
                try:
                    await target_acc["client"].disconnect()
                    if os.path.exists(target_acc["session_path"]):
                        os.remove(target_acc["session_path"])
                    await pool.load_all()
                    await event.answer("✅ Akkaunt o'chirildi!", alert=True)
                except Exception as e:
                    await event.answer(f"Xatolik: {e}", alert=True)
            # Akkauntlar ro'yxatiga qaytish
            btns = []
            for acc in pool.accounts:
                btns.append([Button.inline(f"🗑 {acc['name']}", f"del_{acc['id']}".encode())])
            btns.append([Button.inline("➕ Yangi akkaunt qo'shish", b"menu_add_acc")])
            btns.append([Button.inline("🔙 Asosiy panel", b"refresh_dash")])
            await event.edit(
                f"👥 <b>Ulangan akkauntlar ro'yxati ({len(pool.accounts)} ta):</b>\n\n"
                f"Akkauntni o'chirish uchun ustiga bosing:",
                parse_mode="html",
                buttons=btns
            )

        elif data == b"menu_add_acc":
            admin_states[sender_id] = {"state": "await_phone"}
            await event.respond(
                "➕ <b>Yangi akkaunt qo'shish:</b>\n\n"
                "Iltimos, ulamoqchi bo'lgan Telegram profilingizning telefon raqamini yuboring:\n"
                "<i>Masalan: +998901234567</i>\n\n"
                "Bekor qilish uchun /cancel deb yozing.",
                parse_mode="html"
            )

        elif data == b"menu_channel":
            admin_states[sender_id] = {"state": "await_channel"}
            await event.respond(
                f"📢 <b>Kanalni o'zgartirish:</b>\n\n"
                f"Hozirgi kanal: <code>{cfg.get('channel')}</code>\n\n"
                f"Yangi kanal username yoki havolasini yuboring (masalan: <code>@vibe_ruslan</code>):\n\n"
                f"Bekor qilish uchun /cancel deb yozing.",
                parse_mode="html"
            )

        elif data == b"menu_timing":
            btns = [
                [Button.inline("⏱ Kechikishni sozlash (Sekund)", b"set_delay")],
                [Button.inline("☕️ Tanaffusni sozlash (Batch & Pause)", b"set_pause")],
                [Button.inline("🔄 Yuborish turini almashtirish", b"toggle_mode")],
                [Button.inline("🔙 Asosiy panel", b"refresh_dash")],
            ]
            await event.edit(
                f"⚙️ <b>Vaqt va Tanaffus Sozlamalari:</b>\n\n"
                f"• Tasodifiy kechikish: <b>{cfg.get('min_delay')} – {cfg.get('max_delay')} soniya</b>\n"
                f"• Tanaffus: <b>Har {cfg.get('batch_size')} tadan so'ng {cfg.get('pause_duration')} soniya</b>\n"
                f"• Yuborish turi: <b>{'Forward' if cfg.get('mode') == 'forward' else 'Asl nusxa'}</b>\n\n"
                f"O'zgartirmoqchi bo'lgan parametrni tanlang:",
                parse_mode="html",
                buttons=btns
            )

        elif data == b"toggle_mode":
            cfg["mode"] = "copy" if cfg.get("mode") == "forward" else "forward"
            save_config(cfg)
            await event.answer(f"Yuborish turi: {cfg['mode']} ga o'zgardi!", alert=True)
            # Re-render timing menu
            btns = [
                [Button.inline("⏱ Kechikishni sozlash (Sekund)", b"set_delay")],
                [Button.inline("☕️ Tanaffusni sozlash (Batch & Pause)", b"set_pause")],
                [Button.inline("🔄 Yuborish turini almashtirish", b"toggle_mode")],
                [Button.inline("🔙 Asosiy panel", b"refresh_dash")],
            ]
            await event.edit(
                f"⚙️ <b>Vaqt va Tanaffus Sozlamalari:</b>\n\n"
                f"• Tasodifiy kechikish: <b>{cfg.get('min_delay')} – {cfg.get('max_delay')} soniya</b>\n"
                f"• Tanaffus: <b>Har {cfg.get('batch_size')} tadan so'ng {cfg.get('pause_duration')} soniya</b>\n"
                f"• Yuborish turi: <b>{'Forward' if cfg.get('mode') == 'forward' else 'Asl nusxa'}</b>\n\n"
                f"O'zgartirmoqchi bo'lgan parametrni tanlang:",
                parse_mode="html",
                buttons=btns
            )

        elif data == b"set_delay":
            admin_states[sender_id] = {"state": "await_delay"}
            await event.respond(
                "⏱ <b>Tasodifiy kechikish vaqtini kiriting:</b>\n\n"
                "Min va Max soniyalarni probel bilan ajratib yozing.\n"
                "<i>Masalan: 5 12 (5 dan 12 soniyagacha tasodifiy kutadi)</i>\n\n"
                "Bekor qilish uchun /cancel deb yozing.",
                parse_mode="html"
            )

        elif data == b"set_pause":
            admin_states[sender_id] = {"state": "await_pause"}
            await event.respond(
                "☕️ <b>Tanaffus parametrlarini kiriting:</b>\n\n"
                "Batch va Kutiladigan soniyani probel bilan ajratib yozing.\n"
                "<i>Masalan: 15 60 (har 15 ta xabardan keyin 60 soniya dam oladi)</i>\n\n"
                "Bekor qilish uchun /cancel deb yozing.",
                parse_mode="html"
            )

        elif data == b"menu_manual":
            admin_states[sender_id] = {"state": "await_manual_msg"}
            await event.respond(
                "✍️ <b>Qo'lda xabar yuborish:</b>\n\n"
                "Barcha kanal obunachilarining shaxsiyiga yubormoqchi bo'lgan xabaringizni yuboring (Matn, rasm yoki video):\n\n"
                "Bekor qilish uchun /cancel deb yozing.",
                parse_mode="html"
            )

    # ─────────────────── Message Handlers ───────────────────

    @bot.on(events.NewMessage)
    async def on_message(event):
        sender_id = event.sender_id
        text = event.raw_text.strip()
        print(f"[bot] Xabar keldi: User ID={sender_id}, Text={text}")

        # Faqat admin uchun
        if not is_admin(cfg, sender_id):
            await event.respond(f"🔒 <b>Siz admin emassiz!</b>\n\nSizning Telegram ID: <code>{sender_id}</code>", parse_mode="html")
            return

        if text == "/cancel":
            if sender_id in admin_states:
                del admin_states[sender_id]
            if sender_id in pending_logins:
                try:
                    await pending_logins[sender_id]["client"].disconnect()
                except Exception:
                    pass
                del pending_logins[sender_id]
            await event.respond("🚫 Bekor qilindi.", buttons=get_dashboard_keyboard(cfg))
            return

        if text == "/start" or text == "/panel":
            if sender_id in admin_states:
                del admin_states[sender_id]
            await event.respond(get_dashboard_text(cfg, pool), parse_mode="html", buttons=get_dashboard_keyboard(cfg))
            return

        state_data = admin_states.get(sender_id)
        if not state_data:
            return

        state = state_data.get("state")

        # ── Akkaunt qo'shish: Telefon raqam ──
        if state == "await_phone":
            phone = text.replace(" ", "").replace("-", "")
            safe_phone = phone.replace("+", "")
            session_path = os.path.join(SESSIONS_DIR, f"acc_{safe_phone}")

            client = TelegramClient(session_path, API_ID, API_HASH)
            await client.connect()

            try:
                sent_code = await client.send_code_request(phone)
                pending_logins[sender_id] = {
                    "client": client,
                    "phone": phone,
                    "phone_code_hash": sent_code.phone_code_hash,
                    "session_path": session_path,
                }
                admin_states[sender_id] = {"state": "await_code"}
                await event.respond(
                    f"📩 <b>{phone}</b> raqamiga Telegram orqali tasdiqlash kodi yuborildi!\n\n"
                    f"Iltimos, kelgan kodni kiriting (probel qo'ymasdan):\n\n"
                    f"<i>Bekor qilish: /cancel</i>",
                    parse_mode="html"
                )
            except Exception as e:
                await client.disconnect()
                del admin_states[sender_id]
                await event.respond(f"❌ Xatolik yuz berdi: {e}\n\nQaytadan urinib ko'ring.", buttons=get_dashboard_keyboard(cfg))

        # ── Akkaunt qo'shish: Kod ──
        elif state == "await_code":
            login_data = pending_logins.get(sender_id)
            if not login_data:
                del admin_states[sender_id]
                return

            client = login_data["client"]
            phone = login_data["phone"]
            code = text.strip()

            try:
                await client.sign_in(phone, code)
                me = await client.get_me()
                del pending_logins[sender_id]
                del admin_states[sender_id]
                await pool.load_all()
                u_tag = f"@{me.username}" if me.username else "yo'q"
                await event.respond(
                    f"🎉 <b>Muvaffaqiyatli ulandi!</b>\n\n"
                    f"👤 Ism: <b>{me.first_name}</b>\n"
                    f"🔗 Username: {u_tag}\n"
                    f"🆔 ID: <code>{me.id}</code>\n\n"
                    f"Yangi akkaunt tarqatish havzasiga qo'shildi!",
                    parse_mode="html",
                    buttons=get_dashboard_keyboard(cfg)
                )
            except SessionPasswordNeededError:
                admin_states[sender_id] = {"state": "await_2fa"}
                await event.respond(
                    "🔐 Akkauntingizda <b>2FA (Ikki bosqichli parol)</b> yoqilgan!\n\n"
                    "Iltimos, 2FA parolingizni kiriting:",
                    parse_mode="html"
                )
            except Exception as e:
                await event.respond(f"❌ Kod xato yoki xatolik yuz berdi: {e}\n\nQaytadan kodni kiriting (yoki /cancel):")

        # ── Akkaunt qo'shish: 2FA Parol ──
        elif state == "await_2fa":
            login_data = pending_logins.get(sender_id)
            if not login_data:
                del admin_states[sender_id]
                return

            client = login_data["client"]
            pwd = text.strip()

            try:
                await client.sign_in(password=pwd)
                me = await client.get_me()
                del pending_logins[sender_id]
                del admin_states[sender_id]
                await pool.load_all()
                u_tag = f"@{me.username}" if me.username else "yo'q"
                await event.respond(
                    f"🎉 <b>Muvaffaqiyatli ulandi!</b>\n\n"
                    f"👤 Ism: <b>{me.first_name}</b>\n"
                    f"🔗 Username: {u_tag}\n"
                    f"🆔 ID: <code>{me.id}</code>\n\n"
                    f"Yangi akkaunt tarqatish havzasiga qo'shildi!",
                    parse_mode="html",
                    buttons=get_dashboard_keyboard(cfg)
                )
            except Exception as e:
                await event.respond(f"❌ 2FA parol xato: {e}\n\nQaytadan parolni kiriting (yoki /cancel):")

        # ── Kanalni o'zgartirish ──
        elif state == "await_channel":
            new_ch = text.strip()
            cfg["channel"] = new_ch
            save_config(cfg)
            del admin_states[sender_id]

            # Kanalni qayta sozlaymiz
            await setup_channel_listener(pool, cfg, bot)
            await event.respond(f"✅ Kanal <code>{new_ch}</code> ga muvaffaqiyatli o'zgartirildi!", parse_mode="html", buttons=get_dashboard_keyboard(cfg))

        # ── Kechikishni sozlash ──
        elif state == "await_delay":
            parts = text.split()
            if len(parts) >= 2:
                try:
                    cfg["min_delay"] = int(parts[0])
                    cfg["max_delay"] = int(parts[1])
                    save_config(cfg)
                    del admin_states[sender_id]
                    await event.respond(f"✅ Kechikish <b>{cfg['min_delay']} – {cfg['max_delay']} soniya</b> qilib belgilandi!", parse_mode="html", buttons=get_dashboard_keyboard(cfg))
                    return
                except ValueError:
                    pass
            await event.respond("⚠️ Noto'g'ri format. Iltimos, ikkita sonni probel bilan kiriting (masalan: <code>5 12</code>):", parse_mode="html")

        # ── Tanaffusni sozlash ──
        elif state == "await_pause":
            parts = text.split()
            if len(parts) >= 2:
                try:
                    cfg["batch_size"] = int(parts[0])
                    cfg["pause_duration"] = int(parts[1])
                    save_config(cfg)
                    del admin_states[sender_id]
                    await event.respond(f"✅ Tanaffus: <b>Har {cfg['batch_size']} ta xabardan so'ng {cfg['pause_duration']} soniya</b> qilib belgilandi!", parse_mode="html", buttons=get_dashboard_keyboard(cfg))
                    return
                except ValueError:
                    pass
            await event.respond("⚠️ Noto'g'ri format. Iltimos, ikkita sonni probel bilan kiriting (masalan: <code>15 60</code>):", parse_mode="html")

        # ── Qo'lda xabar yuborish ──
        elif state == "await_manual_msg":
            del admin_states[sender_id]
            admin_acc = pool.accounts[0] if pool.accounts else None
            if not admin_acc:
                await event.respond("❌ Faol akkaunt yo'q!")
                return

            channel = await admin_acc["client"].get_entity(cfg.get("channel"))
            subscribers = await get_channel_subscribers(admin_acc["client"], channel)
            if not subscribers:
                await event.respond("❌ Kanaldagi obunachilarni olib bo'lmadi.")
                return

            await event.respond(f"🚀 Xabaringiz {len(subscribers)} ta obunachiga tarqatilish uchun navbatga qo'yildi...")
            await post_queue.put((channel, [event.message]))

    print("🟢 Tizim to'liq tayyor! Bot buyruqlarni qabul qilmoqda...")
    await bot.run_until_disconnected()


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\n👋 Bot to'xtatildi.")

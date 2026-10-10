import os
import sys
import glob
import json
import random
import asyncio
from datetime import datetime
from telethon import TelegramClient, events
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

os.makedirs(SESSIONS_DIR, exist_ok=True)

# Postlar navbati va media guruh buferi
media_group_buffers = {}
post_queue = asyncio.Queue()


def load_config():
    default_cfg = {
        "channel": "@shshdhdhdwiw",
        "min_delay": 5,
        "max_delay": 12,
        "batch_size": 15,
        "pause_duration": 60,
        "mode": "forward",
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


def load_sent_users():
    if not os.path.exists(SENT_LOG_FILE):
        return set()
    with open(SENT_LOG_FILE, "r", encoding="utf-8") as f:
        return set(line.strip() for line in f if line.strip())


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
        self.accounts.clear()
        session_files = sorted(glob.glob(os.path.join(SESSIONS_DIR, "*.session")))

        if not session_files:
            return []

        print(f"🔄 Akkauntlar yuklanmoqda ({len(session_files)} ta topildi)...")
        for s_path in session_files:
            s_name = os.path.splitext(s_path)[0]
            try:
                client = TelegramClient(s_name, API_ID, API_HASH)
                await client.connect()
                if not await client.is_user_authorized():
                    print(f"⚠️ Avtorizatsiyadan o'tmagan sessiya: {os.path.basename(s_path)}")
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
                print(f"⚠️ Sessiyani ochishda xatolik ({os.path.basename(s_path)}): {e}")

        return self.accounts

    def get_available_account(self):
        """Navbatdagi ishchi akkauntni qaytaradi (Round-robin)"""
        if not self.accounts:
            return None

        now = asyncio.get_event_loop().time()
        for _ in range(len(self.accounts)):
            acc = self.accounts[self.current_idx]
            self.current_idx = (self.current_idx + 1) % len(self.accounts)

            if acc["cooldown_until"] <= now:
                return acc

        # Hamma akkaunt dam olayotgan bo'lsa, eng kam kutadiganini topamiz
        min_cooldown_acc = min(self.accounts, key=lambda a: a["cooldown_until"])
        return min_cooldown_acc

    def set_cooldown(self, acc, seconds):
        now = asyncio.get_event_loop().time()
        acc["cooldown_until"] = now + seconds


async def add_new_account():
    """Yangi Telegram akkauntni login qilib, sessiya sifatida saqlaydi"""
    print("\n➕ Yangi Telegram akkaunt ulash:")
    phone = input("📱 Telefon raqamni kiriting (masalan: +998901234567): ").strip()
    if not phone:
        print("❌ Raqam kiritilmadi!")
        return

    safe_phone = phone.replace("+", "").replace(" ", "").replace("-", "")
    session_path = os.path.join(SESSIONS_DIR, f"acc_{safe_phone}")

    client = TelegramClient(session_path, API_ID, API_HASH)
    await client.connect()

    if not await client.is_user_authorized():
        try:
            await client.send_code_request(phone)
            code = input(f"📩 {phone} raqamiga yuborilgan Telegram kodini kiriting: ").strip()
            try:
                await client.sign_in(phone, code)
            except SessionPasswordNeededError:
                pwd = input("🔐 2FA (Ikki bosqichli parolingizni) kiriting: ").strip()
                await client.sign_in(password=pwd)

            me = await client.get_me()
            print(f"🎉 Muvaffaqiyatli ulandi: {me.first_name} (@{me.username or 'yo\'q'}, ID: {me.id})")
        except Exception as e:
            print(f"❌ Ulanishda xatolik yuz berdi: {e}")
            if os.path.exists(session_path + ".session"):
                os.remove(session_path + ".session")
        finally:
            await client.disconnect()
    else:
        me = await client.get_me()
        print(f"ℹ️ Bu akkaunt allaqachon ulangan: {me.first_name} (ID: {me.id})")
        await client.disconnect()


# ─────────────────────────────────────────────────────────────
# Xabarlarni jo'natish yadrosi (Send Core)
# ─────────────────────────────────────────────────────────────

async def get_channel_subscribers(client, channel):
    """Kanaldagi a'zolarni yuklash"""
    subscribers = []
    try:
        async for user in client.iter_participants(channel):
            if not user.bot and not user.is_self and not user.deleted:
                subscribers.append(user)
    except ChatAdminRequiredError:
        print("\n❌ XATOLIK: Siz bu kanalda Admin emassiz!")
        return None
    except Exception as e:
        print(f"\n⚠️ Obunachilarni yuklashda xatolik: {e}\n")
        return None
    return subscribers


async def send_single_user(client, user_id, messages, mode, channel):
    """Bitta foydalanuvchiga xabar jo'natish (Protected chat bypass bilan)"""
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


async def distribute_messages_multi(pool, channel, messages, cfg):
    """Multi-Account orqali tasodifiy vaqt va tanaffus bilan tarqatuvchi bosh funksiya"""
    post_id = messages[0].id if messages else 0
    now_str = datetime.now().strftime("%H:%M:%S")
    print(f"\n[{now_str}] 📢 Yangi post aniqlandi (Post ID: {post_id}). Tarqatishga tayyorlanmoqda...")

    admin_acc = pool.accounts[0]
    subscribers = await get_channel_subscribers(admin_acc["client"], channel)
    if subscribers is None:
        print("❌ Obunachilarni olib bo'lmadi. Akkaunt kanalda admin ekanligini tekshiring.")
        return

    total = len(subscribers)
    if total == 0:
        print("ℹ️ Kanaldan obunachilar topilmadi.")
        return

    min_delay = cfg.get("min_delay", 5)
    max_delay = cfg.get("max_delay", 12)
    batch_size = cfg.get("batch_size", 15)
    pause_duration = cfg.get("pause_duration", 60)
    mode = cfg.get("mode", "forward")

    print(f"👥 Jami {total} ta obunachi mavjud.")
    print(f"⚡️ {len(pool.accounts)} ta akkaunt orqali tarqatish boshlandi (Kechikish: {min_delay}–{max_delay}s, Tanaffus: har {batch_size} tadan so'ng {pause_duration}s)...")

    success = 0
    failed = 0
    batch_counter = 0

    for idx, user in enumerate(subscribers, start=1):
        # 1. Navbatdagi ishchi akkauntni tanlaymiz
        acc = pool.get_available_account()
        if not acc:
            print("❌ Hech qanday faol akkaunt qolmadi.")
            break

        now_time = asyncio.get_event_loop().time()
        if acc["cooldown_until"] > now_time:
            wait_sec = int(acc["cooldown_until"] - now_time) + 1
            print(f"⏳ Barcha akkauntlar band. {wait_sec} soniya kutilmoqda...")
            await asyncio.sleep(wait_sec)

        client = acc["client"]
        acc_label = acc["name"]
        name = user.first_name or f"ID: {user.id}"
        t_stamp = datetime.now().strftime("%H:%M:%S")

        print(f"[{t_stamp}] [{idx}/{total}] [{acc_label}] 📤 {name} ga...", end=" ", flush=True)

        try:
            await send_single_user(client, user.id, messages, mode, channel)
            success += 1
            batch_counter += 1
            print("✅ Yetkazildi!")
            record_sent_user(user.id)

            # 2. Batch (To'plam) tanaffusi tekshiruvi
            if batch_counter >= batch_size and idx < total:
                print(f"\n☕️ Telegram xavfsizligi uchun KATTA TANAFFUS: {batch_size} ta xabar jo'natildi. {pause_duration} soniya dam olinmoqda...")
                await asyncio.sleep(pause_duration)
                batch_counter = 0
                print("▶️ Tanaffus yakunlandi, tarqatish davom etmoqda...\n")
            else:
                # 3. Tasodifiy kechikish (Random Delay)
                r_delay = round(random.uniform(min_delay, max_delay), 1)
                await asyncio.sleep(r_delay)

        except FloodWaitError as e:
            print(f"\n⚠️ [{acc_label}] FloodWait: {e.seconds} soniya. Bu akkaunt vaqtincha dam oladi.")
            pool.set_cooldown(acc, e.seconds + 5)

        except PeerFloodError:
            print(f"\n❌ [{acc_label}] PeerFlood (SpamBlock) berildi! Akkaunt 2 soatga dam olishga olindi.")
            pool.set_cooldown(acc, 7200)

        except UserPrivacyRestrictedError:
            print("⚠️ Maxfiylik tufayli yuborilmadi (Foydalanuvchi lichkasini yopgan).")
            failed += 1

        except Exception as e:
            print(f"❌ Xatolik: {e}")
            failed += 1

    fin_str = datetime.now().strftime("%H:%M:%S")
    print(f"[{fin_str}] 🏁 Post tarqatish yakunlandi: {success} ta muvaffaqiyatli, {failed} ta xatolik.\n")


# ─────────────────────────────────────────────────────────────
# Avto-kuzatuvchi va Navbat boshqaruvi
# ─────────────────────────────────────────────────────────────

async def queue_worker_multi(pool, channel, cfg):
    while True:
        messages = await post_queue.get()
        try:
            await distribute_messages_multi(pool, channel, messages, cfg)
        except Exception as e:
            print(f"⚠️ Tarqatishda kutilmagan xatolik: {e}")
        finally:
            post_queue.task_done()


async def run_auto_listener(pool, cfg):
    channel_str = cfg.get("channel", "@shshdhdhdwiw")
    admin_acc = pool.accounts[0]
    client = admin_acc["client"]

    try:
        channel = await client.get_entity(channel_str)
    except Exception as e:
        print(f"❌ Kanalni topib bo'lmadi ({channel_str}): {e}")
        return

    print(f"🔍 Kanal tekshirilmoqda: {getattr(channel, 'title', channel_str)}...")
    subscribers = await get_channel_subscribers(client, channel)
    if subscribers is None:
        print("⚠️ ILTIMOS: Akkauntingizni ushbu kanalda Admin qilib tayinlang va qayta urinib ko'ring!")
        return

    print(f"✅ Kanal ulandi! Hozirda {len(subscribers)} ta obunachi mavjud.")

    # Worker ni ishga tushirish
    asyncio.create_task(queue_worker_multi(pool, channel, cfg))

    async def process_media_group(group_id):
        await asyncio.sleep(2.0)
        if group_id in media_group_buffers:
            msgs = media_group_buffers.pop(group_id, [])
            if msgs:
                await post_queue.put(msgs)

    @client.on(events.NewMessage(chats=channel))
    async def handle_new_post(event):
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
            await post_queue.put([msg])

    min_d = cfg.get("min_delay", 5)
    max_d = cfg.get("max_delay", 12)
    b_size = cfg.get("batch_size", 15)
    p_dur = cfg.get("pause_duration", 60)

    print("\n" + "=" * 65)
    print("🟢 MULTI-AKKAUNT AVTO-REJIM ISHGA TUSHDI!")
    print(f"📢 Kuzatilayotgan kanal: {channel_str}")
    print(f"👥 Faol akkauntlar: {len(pool.accounts)} ta")
    print(f"⏱ Tasodifiy kechikish: {min_d} – {max_d} soniya")
    print(f"☕️ Katta tanaffus: Har {b_size} ta xabardan so'ng {p_dur} soniya")
    print("=" * 65)
    print("👉 Kanalingizga post tashlab ko'ring. Barcha akkauntlar navbat bilan tarqatadi!")
    print("*(To'xtatish uchun: Ctrl + C bosing)*\n")

    await client.run_until_disconnected()


async def run_manual_send(pool, cfg):
    """Qo'lda xabar yozib hamma a'zolarga tarqatish"""
    channel_str = cfg.get("channel", "@shshdhdhdwiw")
    admin_acc = pool.accounts[0]
    client = admin_acc["client"]

    try:
        channel = await client.get_entity(channel_str)
    except Exception as e:
        print(f"❌ Kanalni topib bo'lmadi: {e}")
        return

    subscribers = await get_channel_subscribers(client, channel)
    if subscribers is None or len(subscribers) == 0:
        print("❌ Obunachilar topilmadi yoki admin huquqi yo'q.")
        return

    print(f"\n👥 Kanaldagi obunachilar soni: {len(subscribers)} ta")
    print("✍️ Yuboriladigan xabar matnini kiriting (Tugashini bildirish uchun 2 marta Enter bosing):")
    lines = []
    while True:
        line = input()
        if line == "":
            if lines:
                break
        else:
            lines.append(line)
    message_text = "\n".join(lines).strip()

    if not message_text:
        print("❌ Xabar bo'sh bo'lishi mumkin emas!")
        return

    confirm = input(f"❓ {len(subscribers)} ta obunachiga {len(pool.accounts)} ta akkaunt orqali yuborilsinmi? (ha / yo'q): ").strip().lower()
    if confirm not in ("ha", "h", "yes", "y"):
        print("🚫 Bekor qilindi.")
        return

    # Dummy message object
    sent_dummy = await client.send_message("me", message_text)
    await distribute_messages_multi(pool, channel, [sent_dummy], cfg)
    await client.delete_messages("me", [sent_dummy.id])


async def setup_config(cfg):
    print("\n⚙️ Sozlamalarni kiritish:")
    ch = input(f"📢 Kuzatiladigan kanal (Hozirgi: {cfg.get('channel', '@shshdhdhdwiw')}): ").strip()
    if ch:
        cfg["channel"] = ch

    min_d = input(f"⏱ Minimal kechikish soniyada (Hozirgi: {cfg.get('min_delay', 5)}): ").strip()
    if min_d:
        try:
            cfg["min_delay"] = int(min_d)
        except ValueError:
            pass

    max_d = input(f"⏱ Maksimal kechikish soniyada (Hozirgi: {cfg.get('max_delay', 12)}): ").strip()
    if max_d:
        try:
            cfg["max_delay"] = int(max_d)
        except ValueError:
            pass

    b_size = input(f"☕️ Har nechta xabardan keyin tanaffus qilsin? (Hozirgi: {cfg.get('batch_size', 15)}): ").strip()
    if b_size:
        try:
            cfg["batch_size"] = int(b_size)
        except ValueError:
            pass

    p_dur = input(f"☕️ Tanaffus necha soniya bo'lsin? (Hozirgi: {cfg.get('pause_duration', 60)}): ").strip()
    if p_dur:
        try:
            cfg["pause_duration"] = int(p_dur)
        except ValueError:
            pass

    save_config(cfg)
    print("✅ Sozlamalar yangilandi!\n")
    return cfg


# ─────────────────────────────────────────────────────────────
# Asosiy kirish nuqtasi (Main)
# ─────────────────────────────────────────────────────────────

async def main():
    print("\n" + "=" * 65)
    print("🚀 Multi-Account Telegram Avtomatik Tarqatuvchi (Pro Rassilka)")
    print("=" * 65)

    pool = AccountPool()
    accounts = await pool.load_all()

    if not accounts:
        print("\n⚠️ Hali birorta ham akkaunt ulanmagan.")
        await add_new_account()
        accounts = await pool.load_all()
        if not accounts:
            print("❌ Akkaunt ulanmadi. Dastur to'xtatildi.")
            return

    cfg = load_config()

    while True:
        print("\n" + "-" * 50)
        print(f"👥 Ulangan akkauntlar soni: {len(accounts)} ta")
        for i, acc in enumerate(accounts, start=1):
            print(f"   {i}. {acc['name']} ({acc['username']}, ID: {acc['id']})")
        print(f"📢 Kuzatilayotgan kanal: {cfg.get('channel')}")
        print(f"⏱ Kechikish: {cfg.get('min_delay')}–{cfg.get('max_delay')} soniya | ☕️ Tanaffus: Har {cfg.get('batch_size')} tadan keyin {cfg.get('pause_duration')}s")
        print("-" * 50)

        print("\n📋 Menyudan tanlang:")
        print("  1. 🟢 AVTO-REJIM (Kanal postini hamma akkauntlar orqali navbat bilan tarqatish) [Standart]")
        print("  2. ➕ Yangi akkaunt qo'shish (Yana boshqa Telegram profil ulash)")
        print("  3. ✍️ QO'LDA XABAR YUBORISH (Hozir matn kiritib tarqatish)")
        print("  4. ⚙️ Sozlamalarni o'zgartirish (Kanal, kechikish va tanaffus)")
        print("  5. 🚪 Chiqish")

        choice = input("\nTanlang (1–5, Enter = 1): ").strip()

        if choice == "2":
            await add_new_account()
            accounts = await pool.load_all()
        elif choice == "3":
            await run_manual_send(pool, cfg)
        elif choice == "4":
            cfg = await setup_config(cfg)
        elif choice == "5":
            print("👋 Dasturdan chiqildi.")
            break
        else:
            await run_auto_listener(pool, cfg)
            break


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\n\n👋 Dastur to'xtatildi.")

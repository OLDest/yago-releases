package com.yago.mobile;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.ServiceInfo;
import android.media.AudioAttributes;
import android.media.AudioFocusRequest;
import android.media.AudioManager;
import android.media.MediaMetadata;
import android.media.session.MediaSession;
import android.media.session.PlaybackState;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.PowerManager;
import org.json.JSONObject;
import org.gioui.MainActivity;

public final class PlaybackService extends Service {
    static final String UPDATE = "com.yago.mobile.UPDATE";
    private static final String CHANNEL = "playback";
    private static final int NOTICE = 1;
    private MediaSession session;
    private AudioManager audio;
    private AudioFocusRequest focus;
    private PowerManager.WakeLock wake;
    private boolean haveFocus, resumeOnGain, playing, loading;
    private boolean liked, feedbackEnabled;
    private String trackId = "";
    private String title = "YaGo", artist = "Загрузка…";
    private long duration, position;
    private boolean notificationPosted;
    private long wakeRenewAt;
    private final BroadcastReceiver noisy = new BroadcastReceiver() {
        @Override public void onReceive(Context context, Intent intent) { Bridge.command("pause"); }
    };
    @Override public void onCreate() {
        super.onCreate();
        Bridge.initialize(this);
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel(CHANNEL, "Музыка", NotificationManager.IMPORTANCE_LOW));
        audio = (AudioManager) getSystemService(AUDIO_SERVICE);
        AudioAttributes attributes = new AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_MEDIA)
                .setContentType(AudioAttributes.CONTENT_TYPE_MUSIC).build();
        focus = new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN).setAudioAttributes(attributes)
                .setOnAudioFocusChangeListener(new AudioManager.OnAudioFocusChangeListener() {
                    @Override public void onAudioFocusChange(int change) {
                    if (change == AudioManager.AUDIOFOCUS_GAIN) {
                        haveFocus = true;
                        if (resumeOnGain) { resumeOnGain = false; Bridge.command("play"); }
                    } else {
                        haveFocus = false;
                        resumeOnGain = change != AudioManager.AUDIOFOCUS_LOSS && playing;
                        Bridge.command("pause");
                    }
                }}, new Handler(Looper.getMainLooper())).build();
        session = new MediaSession(this, "YaGo");
        session.setFlags(MediaSession.FLAG_HANDLES_MEDIA_BUTTONS | MediaSession.FLAG_HANDLES_TRANSPORT_CONTROLS);
        session.setCallback(new MediaSession.Callback() {
            @Override public void onPlay() { Bridge.command("play"); }
            @Override public void onPause() { Bridge.command("pause"); }
            @Override public void onStop() { Bridge.command("stop"); }
            @Override public void onSkipToNext() { Bridge.command("next"); }
            @Override public void onSkipToPrevious() { Bridge.command("previous"); }
            @Override public void onSeekTo(long pos) { Bridge.command("seek:" + pos); }
            @Override public void onCustomAction(String action, android.os.Bundle extras) {
                if (action != null && (action.startsWith("like:") || action.startsWith("dislike:"))) Bridge.command(action);
            }
            @Override public boolean onMediaButtonEvent(Intent mediaButtonIntent) {
                return super.onMediaButtonEvent(mediaButtonIntent);
            }
        }, new Handler(Looper.getMainLooper()));
        session.setActive(true);
        PowerManager power = (PowerManager) getSystemService(POWER_SERVICE);
        wake = power.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "YaGo:Playback");
        wake.setReferenceCounted(false);
        IntentFilter filter = new IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY);
        if (Build.VERSION.SDK_INT >= 33) registerReceiver(noisy, filter, RECEIVER_NOT_EXPORTED);
        else registerReceiver(noisy, filter);
    }
    @Override public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent == null) { stopSelf(); return START_NOT_STICKY; }
        String action = intent.getAction();
        if (!UPDATE.equals(action)) {
            if ("toggle".equals(action)) Bridge.command("toggle");
            else if ("next".equals(action)) Bridge.command("next");
            else if ("previous".equals(action)) Bridge.command("previous");
            else if ("like".equals(action) || "dislike".equals(action)) {
                String id = intent.getStringExtra("trackId");
                if (id != null && !id.isEmpty()) Bridge.command(action + ":" + id);
            }
            else if ("stop".equals(action)) { Bridge.command("stop"); stopSelf(); }
            return START_NOT_STICKY;
        }
        String oldTitle = title, oldArtist = artist, oldTrackId = trackId;
        long oldDuration = duration;
        boolean oldPlaying = playing, oldLoading = loading, oldLiked = liked, oldFeedback = feedbackEnabled;
        try {
            JSONObject state = new JSONObject(intent.getStringExtra("state"));
            title = state.optString("title", "YaGo"); artist = state.optString("artist", "");
            duration = state.optLong("duration"); position = state.optLong("position");
            playing = state.optBoolean("playing"); loading = state.optBoolean("loading");
            trackId = state.optString("trackId", "");
            liked = state.optBoolean("liked"); feedbackEnabled = state.optBoolean("feedbackEnabled");
        } catch (Exception ignored) { }
        boolean metadataChanged = !notificationPosted || !title.equals(oldTitle) || !artist.equals(oldArtist) || duration != oldDuration || !trackId.equals(oldTrackId);
        boolean controlsChanged = metadataChanged || oldPlaying != playing || oldLoading != loading || oldLiked != liked || oldFeedback != feedbackEnabled;
        if (controlsChanged) {
            Notification notification = notification();
            if (Build.VERSION.SDK_INT >= 29) startForeground(NOTICE, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK);
            else startForeground(NOTICE, notification);
            notificationPosted = true;
        }
        if ((playing || loading) && !haveFocus) {
            haveFocus = audio.requestAudioFocus(focus) == AudioManager.AUDIOFOCUS_REQUEST_GRANTED;
            if (!haveFocus) Bridge.command("pause");
        }
        if (playing || loading) {
            long now = android.os.SystemClock.elapsedRealtime();
            if (!wake.isHeld() || now >= wakeRenewAt) {
                wake.acquire(30 * 60 * 1000L);
                wakeRenewAt = now + 25 * 60 * 1000L;
            }
        } else { if (wake.isHeld()) wake.release(); wakeRenewAt = 0; if (!resumeOnGain && haveFocus) { audio.abandonAudioFocusRequest(focus); haveFocus = false; } }
        if (metadataChanged) session.setMetadata(new MediaMetadata.Builder().putString(MediaMetadata.METADATA_KEY_TITLE, title)
                .putString(MediaMetadata.METADATA_KEY_ARTIST, artist).putLong(MediaMetadata.METADATA_KEY_DURATION, duration).build());
        PlaybackState.Builder playback = new PlaybackState.Builder().setActions(PlaybackState.ACTION_PLAY | PlaybackState.ACTION_PAUSE |
                PlaybackState.ACTION_PLAY_PAUSE | PlaybackState.ACTION_STOP | PlaybackState.ACTION_SKIP_TO_NEXT |
                PlaybackState.ACTION_SKIP_TO_PREVIOUS | PlaybackState.ACTION_SEEK_TO)
                .setState(loading ? PlaybackState.STATE_BUFFERING : playing ? PlaybackState.STATE_PLAYING : PlaybackState.STATE_PAUSED,
                        position, playing ? 1f : 0f);
        if (feedbackEnabled && !trackId.isEmpty()) {
            playback.addCustomAction(new PlaybackState.CustomAction.Builder("like:" + trackId,
                    liked ? "Снять лайк" : "Нравится", likeIcon()).build());
            playback.addCustomAction(new PlaybackState.CustomAction.Builder("dislike:" + trackId,
                    "Не рекомендовать", R.drawable.ic_dislike).build());
        }
        session.setPlaybackState(playback.build());
        return START_NOT_STICKY;
    }
    private PendingIntent control(String action, int request) {
        return PendingIntent.getService(this, request, new Intent(this, PlaybackService.class).setAction(action).putExtra("trackId", trackId),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }
    private Notification notification() {
        PendingIntent open = PendingIntent.getActivity(this, 0, new Intent(this, MainActivity.class)
                .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification.Builder builder = new Notification.Builder(this, CHANNEL).setSmallIcon(com.yago.mobile.R.drawable.ic_music)
                .setContentTitle(title).setContentText(loading ? "Загрузка…" : artist).setContentIntent(open)
                .setOnlyAlertOnce(true).setVisibility(Notification.VISIBILITY_PUBLIC).setOngoing(playing || loading)
                .addAction(new Notification.Action.Builder(R.drawable.ic_previous, "Назад", control("previous", 1)).build())
                .addAction(new Notification.Action.Builder(playing ? R.drawable.ic_pause : R.drawable.ic_play,
                        playing ? "Пауза" : "Играть", control("toggle", 2)).build())
                .addAction(new Notification.Action.Builder(R.drawable.ic_next, "Далее", control("next", 3)).build())
                .setStyle(new Notification.MediaStyle().setMediaSession(session.getSessionToken()).setShowActionsInCompactView(0, 1, 2));
        if (feedbackEnabled && !trackId.isEmpty()) {
            builder.addAction(new Notification.Action.Builder(likeIcon(), liked ? "Снять лайк" : "Нравится", control("like", 4)).build());
            builder.addAction(new Notification.Action.Builder(R.drawable.ic_dislike, "Не рекомендовать", control("dislike", 5)).build());
        }
        return builder.build();
    }
    private int likeIcon() { return liked ? R.drawable.ic_liked : R.drawable.ic_like; }
    @Override public void onTaskRemoved(Intent rootIntent) { Bridge.command("task-removed"); stopSelf(); }
    @Override public void onDestroy() {
        try { unregisterReceiver(noisy); } catch (IllegalArgumentException ignored) { }
        if (wake != null && wake.isHeld()) wake.release();
        if (audio != null && focus != null) audio.abandonAudioFocusRequest(focus);
        if (session != null) { session.setActive(false); session.release(); }
        stopForeground(true);
        super.onDestroy();
    }
    @Override public IBinder onBind(Intent intent) { return null; }
}

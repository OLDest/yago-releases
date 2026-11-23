// GioView lifecycle integration follows GioActivity (Gio authors, MIT).
package org.gioui;

import android.Manifest;
import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.os.Build;
import android.os.Bundle;
import android.widget.FrameLayout;
import org.gioui.GioView;
import com.yago.mobile.Bridge;

public final class MainActivity extends Activity {
    private GioView view;
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        Bridge.initialize(this);
        setVolumeControlStream(android.media.AudioManager.STREAM_MUSIC);
        FrameLayout layer = new FrameLayout(this);
        view = new GioView(this);
        view.setLayoutParams(new FrameLayout.LayoutParams(-1, -1));
        view.setFocusable(true);
        view.setFocusableInTouchMode(true);
        layer.addView(view);
        setContentView(layer);
        view.onIntentEvent(getIntent());
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED)
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 1);
    }
    @Override public void onStart() { super.onStart(); view.start(); }
    @Override public void onStop() { view.stop(); super.onStop(); }
    @Override public void onPause() { super.onPause(); view.pause(); }
    @Override public void onResume() { super.onResume(); view.resume(); }
    @Override public void onDestroy() { view.destroy(); super.onDestroy(); }
    @Override public void onConfigurationChanged(Configuration c) { super.onConfigurationChanged(c); view.configurationChanged(); }
    @Override public void onLowMemory() { super.onLowMemory(); GioView.onLowMemory(); }
    @Override public void onBackPressed() {
        if (!view.backPressed()) moveTaskToBack(true);
    }
    @Override protected void onNewIntent(Intent intent) { super.onNewIntent(intent); setIntent(intent); view.onIntentEvent(intent); }
}

package com.yago.mobile;

import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import org.json.JSONObject;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.util.Arrays;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

public final class Bridge {
    private static final String ALIAS = "yago.credentials.v1";
    private static Context context;
    private static final Handler main = new Handler(Looper.getMainLooper());
    private static boolean serviceRequested;
    public static void initialize(Context value) { context = value.getApplicationContext(); }
    public static native void nativeCommand(String command);
    static void command(String command) {
        try { nativeCommand(command); } catch (UnsatisfiedLinkError ignored) { }
    }
    private static SharedPreferences prefs() {
        return context.getSharedPreferences("credentials", Context.MODE_PRIVATE);
    }
    private static SecretKey key() throws Exception {
        KeyStore store = KeyStore.getInstance("AndroidKeyStore");
        store.load(null);
        if (store.containsAlias(ALIAS)) return (SecretKey) store.getKey(ALIAS, null);
        KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
        generator.init(new KeyGenParameterSpec.Builder(ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256).build());
        return generator.generateKey();
    }
    public static String credentialSave(String json) throws Exception { return saveCredential("token",json); }
    public static String zvukCredentialSave(String json) throws Exception { return saveCredential("zvuk-token",json); }
    public static String zvukCredentialRead(String ignored) throws Exception { return readCredential("zvuk-token"); }
    public static String zvukCredentialDelete(String ignored) { if(!prefs().edit().remove("zvuk-token").commit())throw new IllegalStateException("Credential delete failed");return ""; }
    public static String zvukAuthStart(String url) { return ZvukAuthBridge.start(context,url); }
    public static String zvukAuthPoll(String id) { return ZvukAuthBridge.poll(id); }
    public static String zvukAuthCancel(String id) { ZvukAuthBridge.cancel(context,id);return ""; }
public static String updateCredentialSave(String json) throws Exception { return saveCredential("update-token",json); }
private static String saveCredential(String name,String json) throws Exception {
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, key());
        byte[] ciphertext = cipher.doFinal(json.getBytes(StandardCharsets.UTF_8));
        String sealed = Base64.encodeToString(cipher.getIV(), Base64.NO_WRAP) + ":" +
                Base64.encodeToString(ciphertext, Base64.NO_WRAP);
        if (!prefs().edit().putString(name, sealed).commit()) throw new IllegalStateException("Credential write failed");
        return "";
    }
    public static String credentialRead(String ignored) throws Exception { return readCredential("token"); }
public static String updateCredentialRead(String ignored) throws Exception { return readCredential("update-token"); }
private static String readCredential(String name) throws Exception {
        String sealed = prefs().getString(name, null);
        if (sealed == null) return "";
        String[] fields = sealed.split(":", 2);
        if (fields.length != 2) throw new IllegalStateException("Invalid credential");
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, Base64.decode(fields[0], Base64.NO_WRAP)));
        return new String(cipher.doFinal(Base64.decode(fields[1], Base64.NO_WRAP)), StandardCharsets.UTF_8);
    }
    public static String credentialDelete(String ignored) {
        if (!prefs().edit().remove("token").commit()) throw new IllegalStateException("Credential delete failed");
        return "";
    }
    public static String updateCredentialDelete(String ignored) { if (!prefs().edit().remove("update-token").commit()) throw new IllegalStateException("Credential delete failed"); return ""; }
public static String installUpdate(String json) throws Exception { return UpdateProvider.install(context,json); }
public static String updateUnmetered(String ignored) { android.net.ConnectivityManager c=(android.net.ConnectivityManager)context.getSystemService(Context.CONNECTIVITY_SERVICE); android.net.NetworkCapabilities n=c.getNetworkCapabilities(c.getActiveNetwork());return Boolean.toString(n!=null && n.hasCapability(android.net.NetworkCapabilities.NET_CAPABILITY_NOT_METERED) && n.hasTransport(android.net.NetworkCapabilities.TRANSPORT_WIFI)); }
public static String openURL(String raw) {
        if (!"https://ya.ru/device".equals(raw) && !"https://github.com/login/device".equals(raw)) throw new IllegalArgumentException("Unsupported URL");
        Intent intent = new Intent(Intent.ACTION_VIEW, Uri.parse(raw)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        context.startActivity(intent);
        return "";
    }
    public static String publish(String json) throws Exception {
        JSONObject state = new JSONObject(json);
        final boolean active = state.optBoolean("active");
        final String value = json;
        main.post(new Runnable() { @Override public void run() {
            if (!active) {
                if (serviceRequested) context.stopService(new Intent(context, PlaybackService.class));
                serviceRequested = false;
                return;
            }
            Intent intent = new Intent(context, PlaybackService.class).setAction(PlaybackService.UPDATE).putExtra("state", value);
            try {
                if (!serviceRequested && Build.VERSION.SDK_INT >= 26) context.startForegroundService(intent);
                else context.startService(intent);
                serviceRequested = true;
            } catch (RuntimeException error) {
                serviceRequested = false;
                command("service-error");
            }
        }});
        return "";
    }
}

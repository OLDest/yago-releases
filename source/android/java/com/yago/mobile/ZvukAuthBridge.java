package com.yago.mobile;

import android.content.*;
import android.os.*;
import java.util.UUID;

/** Private, bounded, in-memory handoff from the disposable login process. */
public final class ZvukAuthBridge {
    static final String PERMISSION="com.yago.mobile.INTERNAL_AUTH";
    static final String RESULT="com.yago.mobile.ZVUK_AUTH_RESULT";
    static final String CANCEL="com.yago.mobile.ZVUK_AUTH_CANCEL";
    private static String active="",result="";
    private static boolean registered;
    private static final Handler main=new Handler(Looper.getMainLooper());
    static void register(Context c,BroadcastReceiver r,String action){
        if(Build.VERSION.SDK_INT>=33)c.registerReceiver(r,new IntentFilter(action),PERMISSION,main,Context.RECEIVER_NOT_EXPORTED);
        else c.registerReceiver(r,new IntentFilter(action),PERMISSION,main);
    }
    public static synchronized String start(final Context c,String url){
        if(!"https://id.zvuk.com/?redirect_url=https%3A%2F%2Fzvuk.com%2F".equals(url))throw new IllegalArgumentException("Invalid login origin");
        if(!registered){register(c,new BroadcastReceiver(){@Override public void onReceive(Context context,Intent i){
            synchronized(ZvukAuthBridge.class){if(active.equals(i.getStringExtra("id"))){String body=i.getStringExtra("result");if(body!=null&&body.length()<=65536)result=body;}}
        }},RESULT);registered=true;}
        if(!active.isEmpty())cancel(c,active);
        final String id=UUID.randomUUID().toString();active=id;result="";
        main.post(new Runnable(){@Override public void run(){try{c.startActivity(new Intent(c,ZvukAuthActivity.class).putExtra("id",id).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));}
            catch(RuntimeException e){synchronized(ZvukAuthBridge.class){if(active.equals(id))result="{\"error\":\"unavailable\"}";}}}});
        return id;
    }
    public static synchronized String poll(String id){if(!active.equals(id))return "{\"error\":\"cancelled\"}";String value=result;result="";return value;}
    public static synchronized void cancel(Context c,String id){
        if(!active.equals(id))return;
        active="";result="";
        c.sendBroadcast(new Intent(CANCEL).setPackage(c.getPackageName()).putExtra("id",id),PERMISSION);
    }
}

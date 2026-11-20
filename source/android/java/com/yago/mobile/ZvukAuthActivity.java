package com.yago.mobile;

import android.app.Activity;
import android.content.*;
import android.net.Uri;
import android.os.*;
import android.webkit.*;
import android.widget.*;
import org.json.*;

/** Only login uses WebView. Its private process exits after the last login closes. */
public final class ZvukAuthActivity extends Activity {
    private static int instances;
    private static boolean webInitialized;
    private String id;
    private WebView web;
    private boolean sent;
    private BroadcastReceiver cancelReceiver;
    private final Handler main=new Handler(Looper.getMainLooper());
    private static final String PROFILE=
        "(function(){if(location.origin!=='https://zvuk.com')return '';"+
        "if(window.__yagoProfile)return window.__yagoProfile;"+
        "if(!window.__yagoBusy){window.__yagoBusy=true;(async function(){try{"+
        "for(const p of ['/api/v2/tiny/profile','/api/tiny/profile']){try{const r=await fetch(p,{credentials:'include'});"+
        "if(!r.ok)continue;const x=await r.json();const a=x.result&&(x.result.profile||x.result);"+
        "if(a&&a.id&&a.is_anonymous===false){window.__yagoProfile=JSON.stringify({result:a});break;}}catch(e){}}"+
        "}finally{window.__yagoBusy=false;}})();}return '';})()";
    private final Runnable poll=new Runnable(){@Override public void run(){
        if(web==null||isFinishing())return;
        Uri page=Uri.parse(web.getUrl()==null?"":web.getUrl());
        if("https".equals(page.getScheme())&&"zvuk.com".equals(page.getHost())){
            web.evaluateJavascript(PROFILE,new ValueCallback<String>(){@Override public void onReceiveValue(String value){
                if(web==null||isFinishing())return;
                Uri current=Uri.parse(web.getUrl()==null?"":web.getUrl());
                if(!"https".equals(current.getScheme())||!"zvuk.com".equals(current.getHost()))return;
                try{
                    Object decoded=new JSONTokener(value).nextValue();if(!(decoded instanceof String))return;
                    String body=(String)decoded;if(body.isEmpty()||body.length()>65536)return;
                    JSONObject env=new JSONObject(body),p=env.getJSONObject("result");
                    if(!Boolean.FALSE.equals(p.opt("is_anonymous"))||p.optString("id").isEmpty())return;
                    String token=p.optString("token");
                    if(token.isEmpty()){
                        String cookies=CookieManager.getInstance().getCookie("https://zvuk.com/");
                        if(cookies!=null)for(String item:cookies.split(";")){String[] kv=item.trim().split("=",2);if(kv.length==2&&"auth".equals(kv[0]))token=kv[1];}
                    }
                    if(token.isEmpty()||token.length()>8192)return;
                    p.put("token",token);complete(env.toString());
                }catch(Exception ignored){}
            }});
        }
        main.postDelayed(this,1500);
    }};
    @Override public void onCreate(Bundle state){
        super.onCreate(state);instances++;id=getIntent().getStringExtra("id");
        if(id==null||id.length()>100){finish();return;}
        cancelReceiver=new BroadcastReceiver(){@Override public void onReceive(Context c,Intent i){if(id.equals(i.getStringExtra("id")))finish();}};
        ZvukAuthBridge.register(this,cancelReceiver,ZvukAuthBridge.CANCEL);
        if(!webInitialized){if(Build.VERSION.SDK_INT>=28)WebView.setDataDirectorySuffix("zvuk-auth");webInitialized=true;}
        LinearLayout root=new LinearLayout(this);root.setOrientation(LinearLayout.VERTICAL);
        Button close=new Button(this);close.setText("Закрыть вход в Звук");close.setOnClickListener(new android.view.View.OnClickListener(){@Override public void onClick(android.view.View v){finish();}});root.addView(close);
        final TextView notice=new TextView(this);
        notice.setText("YaGo — неофициальный клиент, не связанный со Звуком.\n\nПосле входа YaGo получит токен вашего аккаунта для воспроизведения музыки и доступа к вашей библиотеке. Токен хранится зашифрованным на устройстве и используется для запросов к Звуку.\n\nВход откроется на сайте id.zvuk.com. Адрес сайта будет виден над страницей. Подключение можно удалить в «Профилях».");
        int padding=(int)(16*getResources().getDisplayMetrics().density);notice.setPadding(padding,padding,padding,padding);
        root.addView(notice);
        final Button consent=new Button(this);consent.setText("Продолжить вход в Звук");root.addView(consent);setContentView(root);
        consent.setOnClickListener(new android.view.View.OnClickListener(){@Override public void onClick(android.view.View v){
            consent.setEnabled(false);root.removeView(notice);root.removeView(consent);beginLogin(root);
        }});
        main.postDelayed(new Runnable(){@Override public void run(){finish();}},10*60*1000);
    }
    private void beginLogin(LinearLayout root){
        final TextView origin=new TextView(this);origin.setText("Сайт входа: https://id.zvuk.com");root.addView(origin);
        WebView.setWebContentsDebuggingEnabled(false);
        web=new WebView(this);WebSettings settings=web.getSettings();settings.setJavaScriptEnabled(true);settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);settings.setAllowContentAccess(false);settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        CookieManager.getInstance().setAcceptCookie(true);CookieManager.getInstance().setAcceptThirdPartyCookies(web,true);
        web.setWebViewClient(new WebViewClient(){
            @Override public boolean shouldOverrideUrlLoading(WebView view,WebResourceRequest r){
                Uri u=r.getUrl();return !"https".equals(u.getScheme())||u.getHost()==null||u.getUserInfo()!=null||(u.getPort()!=-1&&u.getPort()!=443);
            }
            @Override public void onPageStarted(WebView view,String url,android.graphics.Bitmap favicon){
                Uri u=Uri.parse(url);origin.setText("Сайт входа: "+u.getScheme()+"://"+u.getHost());
            }
        });
        root.addView(web,new LinearLayout.LayoutParams(-1,0,1));setContentView(root);
        web.loadUrl("https://id.zvuk.com/?redirect_url=https%3A%2F%2Fzvuk.com%2F");
        main.post(poll);
    }
    private void complete(String body){if(sent)return;sent=true;
        sendBroadcast(new Intent(ZvukAuthBridge.RESULT).setPackage(getPackageName()).putExtra("id",id).putExtra("result",body),ZvukAuthBridge.PERMISSION);finish();
    }
    @Override public void onBackPressed(){if(web!=null&&web.canGoBack())web.goBack();else finish();}
    @Override public void onDestroy(){
        main.removeCallbacksAndMessages(null);
        if(!sent&&id!=null)complete("{\"error\":\"cancelled\"}");
        if(cancelReceiver!=null)unregisterReceiver(cancelReceiver);
        if(web!=null){web.stopLoading();web.loadUrl("about:blank");web.clearHistory();web.clearCache(true);web.removeAllViews();web.destroy();web=null;}
        instances--;super.onDestroy();
        if(instances==0){CookieManager.getInstance().removeAllCookies(null);WebStorage.getInstance().deleteAllData();
            new Handler(Looper.getMainLooper()).postDelayed(new Runnable(){@Override public void run(){if(instances==0)android.os.Process.killProcess(android.os.Process.myPid());}},500);}
    }
}

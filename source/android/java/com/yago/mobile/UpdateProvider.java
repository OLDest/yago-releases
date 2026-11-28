package com.yago.mobile;

import android.content.*;
import android.content.pm.*;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.Build;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.provider.Settings;
import org.json.JSONObject;
import java.io.*;
import java.security.MessageDigest;
import java.util.*;

public final class UpdateProvider extends ContentProvider {
    private static File root(Context c) throws IOException { return new File(c.getFilesDir(),"config/yago/updates").getCanonicalFile(); }
    private static File checked(Context c,File input) throws IOException {
        File f=input.getCanonicalFile();
        if (!f.getPath().startsWith(root(c).getPath()+File.separator) || !f.getName().equals("YaGo-android.apk") || !f.isFile())
            throw new SecurityException("Invalid update path");
        return f;
    }
    private File file(Uri uri) throws IOException {
        if (!"content".equals(uri.getScheme()) || !"com.yago.mobile.updates".equals(uri.getAuthority())) throw new SecurityException("Invalid URI");
        return checked(getContext(),new File(root(getContext()),uri.getPath().substring(1)));
    }
    @Override public boolean onCreate(){return true;}
    @Override public String getType(Uri u){return "application/vnd.android.package-archive";}
    @Override public ParcelFileDescriptor openFile(Uri u,String mode) throws FileNotFoundException {
        if (!"r".equals(mode)) throw new SecurityException("Read only");
        try {return ParcelFileDescriptor.open(file(u),ParcelFileDescriptor.MODE_READ_ONLY);}
        catch(IOException e){throw new FileNotFoundException("Update unavailable");}
    }
    @Override public Cursor query(Uri u,String[] projection,String selection,String[] args,String sort){
        try {
            File f=file(u);
            String[] cols=projection==null?new String[]{OpenableColumns.DISPLAY_NAME,OpenableColumns.SIZE}:projection;
            MatrixCursor result=new MatrixCursor(cols);Object[] row=new Object[cols.length];
            for(int i=0;i<cols.length;i++){if(OpenableColumns.DISPLAY_NAME.equals(cols[i]))row[i]=f.getName();else if(OpenableColumns.SIZE.equals(cols[i]))row[i]=f.length();}
            result.addRow(row);return result;
        }catch(IOException e){throw new IllegalArgumentException("Update unavailable");}
    }
    @Override public Uri insert(Uri u,ContentValues v){throw new UnsupportedOperationException();}
    @Override public int update(Uri u,ContentValues v,String s,String[] a){throw new UnsupportedOperationException();}
    @Override public int delete(Uri u,String s,String[] a){throw new UnsupportedOperationException();}
    private static Set<String> signatures(PackageInfo p) {
        Signature[] s=Build.VERSION.SDK_INT>=28?p.signingInfo.getApkContentsSigners():p.signatures;
        Set<String> result=new HashSet<>();for(Signature x:s)result.add(x.toCharsString());return result;
    }
    public static String install(Context c,String json) throws Exception {
        JSONObject value=new JSONObject(json);File apk=checked(c,new File(value.getString("path")));
        MessageDigest digest=MessageDigest.getInstance("SHA-256");
        try(InputStream in=new FileInputStream(apk)){byte[] b=new byte[65536];int n;while((n=in.read(b))!=-1)digest.update(b,0,n);}
        StringBuilder hash=new StringBuilder();for(byte b:digest.digest())hash.append(String.format(java.util.Locale.ROOT,"%02x",b&255));
        if(!hash.toString().equals(value.getString("sha256")))throw new SecurityException("Update hash mismatch");
        PackageManager pm=c.getPackageManager();int flags=Build.VERSION.SDK_INT>=28?PackageManager.GET_SIGNING_CERTIFICATES:PackageManager.GET_SIGNATURES;
        PackageInfo incoming=pm.getPackageArchiveInfo(apk.getPath(),flags),current=pm.getPackageInfo(c.getPackageName(),flags);
        if(incoming==null||!c.getPackageName().equals(incoming.packageName)||!signatures(incoming).equals(signatures(current)))
            throw new SecurityException("Update signer mismatch");
        long next=Build.VERSION.SDK_INT>=28?incoming.getLongVersionCode():incoming.versionCode;
        long old=Build.VERSION.SDK_INT>=28?current.getLongVersionCode():current.versionCode;
        if(next<=old)throw new SecurityException("Update is not newer");
        if(!pm.canRequestPackageInstalls()){
            c.startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,Uri.parse("package:"+c.getPackageName())).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));return "";
        }
        String relative=root(c).toURI().relativize(apk.toURI()).getPath();
        Uri uri=new Uri.Builder().scheme("content").authority("com.yago.mobile.updates").path("/"+relative).build();
        Intent intent=new Intent(Intent.ACTION_VIEW).setDataAndType(uri,"application/vnd.android.package-archive")

            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK|Intent.FLAG_GRANT_READ_URI_PERMISSION);
        intent.setClipData(ClipData.newRawUri("YaGo update",uri));c.startActivity(intent);return "";
    }
}

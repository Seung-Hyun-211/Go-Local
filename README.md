# Go

HTTP 서버로 만들어 실행는 도중 다른 프로세스로부터 요청을 받을 경우 영상을 다운로드, 변환하는 프로그램
처음 실행하여 초기화하면 youtubeservice v3 apikey를 이용해 base service client를 초기화 한 후 계속 검사하여 요청이 있을 때 마다 해당 클라이언트를 이용해 요청을 처리한다.

apiKey 와 앱 이름은 실행파일 위치에 config.json으로 불러오며, 아래와 같은 구조를 가진다.
```
{
    ApiKey = key,
    ApplicationName = "appname"
}
```

목적은 서버로서 요청을 받고  youtube영상리스트 중 첫 번째의 영상을 pcm파일로 변환후 "db/채널명/노래제목.pcm"로 저장
검색어가 아닌 url이 들어온 경우에는 해당 url의 영상을 pcm파일로 저장한다. 요청을 한 프로세스에 응답을 json형태로 전송한다.
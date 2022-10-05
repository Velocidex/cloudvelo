package testsuite

const SERVER_CONFIG = `
version:
  name: velociraptor
  version: 0.6.4-rc4
  commit: f3264824
  build_time: "2022-04-14T02:23:05+10:00"
Client:
  server_urls:
  - https://localhost:8100/
  ca_certificate: |
    -----BEGIN CERTIFICATE-----
    MIIDTDCCAjSgAwIBAgIRAJH2OrT69FpC7IT3ZeZLmXgwDQYJKoZIhvcNAQELBQAw
    GjEYMBYGA1UEChMPVmVsb2NpcmFwdG9yIENBMB4XDTIxMDQxMzEwNDY1MVoXDTMx
    MDQxMTEwNDY1MVowGjEYMBYGA1UEChMPVmVsb2NpcmFwdG9yIENBMIIBIjANBgkq
    hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAsLO3/Kq7RAwEhHrbsprrvCsE1rpOMQ6Q
    rJHM+0zZbxXchhrYEvi7W+Wae35ptAJehICmbIHwRhgCF2HSkTvNdVzSL9bUQT3Q
    XANxxXNrMW0grOJwQjFYBl8Bo+nv1CcJN7IF2vWcFpagfVHX2dPysfCwzzYX+Ai6
    OK5MqWwk22TJ5NWtUkH7+bMyS+hQbocr/BwKNWGdRlP/+BuUo6N99bVSXqw3gkz8
    FLYHVAKD2K4KaMlgfQtpgYeLKsebjUtKEub9LzJSgEdEFm2bG76LZPbKSGqBLwbv
    x+bJcn23vb4VJrWtbtB0GMxB1bHLTkWgD6PV6ejArClJPvDc9rDrOwIDAQABo4GM
    MIGJMA4GA1UdDwEB/wQEAwICpDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUH
    AwIwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUO2IRSDwqgkZt5pkXdScs5Bjo
    ULEwKAYDVR0RBCEwH4IdVmVsb2NpcmFwdG9yX2NhLnZlbG9jaWRleC5jb20wDQYJ
    KoZIhvcNAQELBQADggEBABRNDOPkGRp/ScFyS+SUY2etd1xLPXbX6R9zxy5AEIp7
    xEVSBcVnzGWH8Dqm2e4/3ZiV+IS5blrSQCfULwcBcaiiReyWXONRgnOMXKm/1omX
    aP7YUyRKIY+wASKUf4vbi+R1zTpXF4gtFcGDKcsK4uQP84ZtLKHw1qFSQxI7Ptfa
    WEhay5yjJwZoyiZh2JCdzUnuDkx2s9SoKi+CL80zRa2rqwYbr0HMepFZ0t83fIzt
    zNezVulkexf3I4keCaKkoT6nPqGd7SDOLhOQauesz7ECyr4m0yL4EekAsMceUvGi
    xdg66BlldhWSiEBcYmoNn5kmWNhV0AleVItxQkuWwbI=
    -----END CERTIFICATE-----
  nonce: rKNKAYam310=
  writeback_darwin: /etc/velociraptor.writeback.yaml
  writeback_linux: /tmp/velociraptor.writeback.yaml
  writeback_windows: $ProgramFiles\Velociraptor\velociraptor.writeback.yaml
  use_self_signed_ssl: true
  max_poll: 10
  windows_installer:
    service_name: Velociraptor
    install_path: $ProgramFiles\Velociraptor\Velociraptor.exe
    service_description: Velociraptor service
  darwin_installer:
    service_name: com.velocidex.velociraptor
    install_path: /usr/local/sbin/velociraptor
  version:
    name: velociraptor
    version: 0.6.4-rc4
    commit: f3264824
    build_time: "2022-04-14T02:23:05+10:00"
  pinned_server_name: VelociraptorServer
  max_upload_size: 5242880
  local_buffer:
    memory_size: 52428800
    disk_size: 1073741824
    filename_linux: /var/tmp/Velociraptor_Buffer.bin
    filename_windows: $TEMP/Velociraptor_Buffer.bin
    filename_darwin: /var/tmp/Velociraptor_Buffer.bin
API:
  bind_address: 127.0.0.1
  bind_port: 8001
  bind_scheme: tcp
  pinned_gw_name: GRPC_GW
GUI:
  initial_users:
    - name: admin
      password_hash: 5058b4b63149663ff0b4e8b4924b999248a43363cd64b302b5b34ea8e68283be
      password_salt: c85fecf44ca8c1352a2c9bf0987c01426f734fe7228a0d14611d35c9cda880cd
  initial_orgs:
    - name: MyOrg
      org_id: O123
  bind_address: 0.0.0.0
  bind_port: 8889
  gw_certificate: |
    -----BEGIN CERTIFICATE-----
    MIIDQjCCAiqgAwIBAgIRALDNncaXLdtho9zXN+h6ehAwDQYJKoZIhvcNAQELBQAw
    GjEYMBYGA1UEChMPVmVsb2NpcmFwdG9yIENBMB4XDTIyMDQxMzE3MDg1M1oXDTIz
    MDQxMzE3MDg1M1owKTEVMBMGA1UEChMMVmVsb2NpcmFwdG9yMRAwDgYDVQQDDAdH
    UlBDX0dXMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAr8nEaDof7O20
    ZaKenYBAW2hWi0bc6N1q8ts0GZFe/5dNc8BK2lhYSg7B+wHR6AHjznQcWqAY8T9r
    VyJxQlIUxLBWDbSSN4lk9FE/SnxcVkdpGFd1kRATQg/eKNwLYaoOiWUMXtKgq3tS
    s9EJxVFDJhjzII8u2QYIBcc2lhYOYuCJTtd0dxzy7umKKKCZDaP7piBbFr48D40q
    LqiGwsnoPskMnVvHXuAYZ5WOmEggUw73Dk8B4KJ8gVH24FmLIAzQu2l2Lk28QNkp
    gu7lYoih3OVnnEXaypqfVcDUJejCAh1TsXp/AR9TPkEo6j9gTVGQM/P1j/sLYSOL
    6RGUDxK7IQIDAQABo3QwcjAOBgNVHQ8BAf8EBAMCBaAwHQYDVR0lBBYwFAYIKwYB
    BQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYDVR0jBBgwFoAUO2IRSDwq
    gkZt5pkXdScs5BjoULEwEgYDVR0RBAswCYIHR1JQQ19HVzANBgkqhkiG9w0BAQsF
    AAOCAQEArYY/cTY7XUEn6ADIvge9zg8N8vqWuyE23RkE07J0H+fgY8N24wzCKOyJ
    vY6YBUTzozC4/Bp7f3aye767IyHWz8fPyB5P8JCrUnOg3gJtrWD85b5HDbPuDUnq
    qroOUZnQIGv+bJUArP8s+/I7iXVOcS7pBNeecWUaRyu09l53mnZlomTMiVQBZfUS
    F8aQBWbf/H9mDitMhcP41GXsi3ygse2JCYYY2pjo6OPoSGxL40G+Fm5TmDRYzQQz
    5GX5MFM7DzbnEPE5CBYBDdGFU/3SlEJSTT9LUqTFO00UTtiAKQqxw4G6XcSLwh/u
    Fy1xpxSoc+VTIoshSn+GMie5+t+OOA==
    -----END CERTIFICATE-----
  gw_private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEAr8nEaDof7O20ZaKenYBAW2hWi0bc6N1q8ts0GZFe/5dNc8BK
    2lhYSg7B+wHR6AHjznQcWqAY8T9rVyJxQlIUxLBWDbSSN4lk9FE/SnxcVkdpGFd1
    kRATQg/eKNwLYaoOiWUMXtKgq3tSs9EJxVFDJhjzII8u2QYIBcc2lhYOYuCJTtd0
    dxzy7umKKKCZDaP7piBbFr48D40qLqiGwsnoPskMnVvHXuAYZ5WOmEggUw73Dk8B
    4KJ8gVH24FmLIAzQu2l2Lk28QNkpgu7lYoih3OVnnEXaypqfVcDUJejCAh1TsXp/
    AR9TPkEo6j9gTVGQM/P1j/sLYSOL6RGUDxK7IQIDAQABAoIBAQCZ74gs9WlroyTs
    M6HOrras1QukX2OoD+1NyiMvmJumGqraiDOETQTTpWS9F/TmxHDnI8qImdX9vNmU
    rjacKyDAtOJGki/QrmJXiKZx5cE+VL51cHElnPwgR6D2Cut0lOSx8GkKyEumnxHn
    IVD7F5RD0mllw7z0k1GHLdJhT+M9NYudY5VlG3zru4z0KvzUiZbj7S1WaXtaV0wv
    Agx8LDRSQA87b50n7kFFE6FR9wSUKQSFdTgW47rLHp2NDnoCziomjY6j50Hp716M
    DYIH/kVnX4MrV1e9HWAYojPrXC7uf0QBXih6wPTSR746jKSGSB+39gjF6/EKBa7S
    RHIG572ZAoGBANnW1K9kSTnCyuaY2msfvtJiWTzu8ufRc6hmpXXij/POUlJe9zUl
    CIcebav1mqOqcwUKKIjJGkaw01wes0CR6sQdZZxpAx10QA+RGnDq6rqN5h+XrAyO
    QOIdUDzij3UwV+Y33prc4QSDAjiwrfzEms8MKbJL8fsMfW+3G/K4YaYrAoGBAM6V
    H/4tKwTtdBynOpKZw6w5cAV0MY+EalLfAqYcwU7vpVFEhup/F7vhLDXdeislt0lW
    Pw20UVzeXG64yBSulpy7B16KpH7WXsVfSVfLyfyou85lvmoOHWOoiTq22psTNXp2
    N9RLBvzc7julLJS4/BnFfrsUn7KVp+9tftBCXqnjAoGAU8XFJuoHKvpZMxLnNDkS
    FjASJ1ew/CtVMha/XLVTLKxUhi7VHI/wVp4CCRY7cONUtkRw0CGeRD8uGQgJYTR8
    Nw0jDWJo+0PAevwPUgtVV4bIT6/xTybJlus09yUdjDCaLQOhTKbStfx3tztivYkS
    C7uesV15YlUsS/D8A3yauN8CgYEAsN7O4IFtlq9zPWEUbhYGiTs4JQNBt53opoFX
    tD9kZqAZy8W/OaCNAogcoRW6Fp0ZG0ojfClJjBi5zPaaH9MHErOy8IgFpK8HvzcB
    BZFuo8sX2PQVnpntIblXnRSXgDRnEi2LSVDfb7n8osadr8vd1HbaNXTH8k/d08nM
    zKQ2hn8CgYAdMY8yqgcl0wcXWiI4C1s7JB0xzn7gUmTwTuYxNs7nj1s5Lj3PoVjC
    aWmjCDLIyNUMK5X8P10nPo6Oy0TqynCy/PMS6SbH+/nZyXXIzz7DZRzIrx+5j4Yd
    95fBApSHHobU8O2OMdw92NlFy25ZRFMP8EUyj7OkDkZ6sQDyeShWuA==
    -----END RSA PRIVATE KEY-----
  internal_cidr:
  - 127.0.0.1/12
  - 192.168.0.0/16
  authenticator:
    type: basic

CA:
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEowIBAAKCAQEAsLO3/Kq7RAwEhHrbsprrvCsE1rpOMQ6QrJHM+0zZbxXchhrY
    Evi7W+Wae35ptAJehICmbIHwRhgCF2HSkTvNdVzSL9bUQT3QXANxxXNrMW0grOJw
    QjFYBl8Bo+nv1CcJN7IF2vWcFpagfVHX2dPysfCwzzYX+Ai6OK5MqWwk22TJ5NWt
    UkH7+bMyS+hQbocr/BwKNWGdRlP/+BuUo6N99bVSXqw3gkz8FLYHVAKD2K4KaMlg
    fQtpgYeLKsebjUtKEub9LzJSgEdEFm2bG76LZPbKSGqBLwbvx+bJcn23vb4VJrWt
    btB0GMxB1bHLTkWgD6PV6ejArClJPvDc9rDrOwIDAQABAoIBAAo6vUIBWEn+MBzD
    SAi080S3cNZFftVUNIfpAObjcgr+Rv/0eeHPSHlvd1wC23eyU2p0UC4j75b/OM/F
    t/z0a1aKAxkF5M/KFk/dWy7FGcWIvcWEbl9GoAPuaBfnKR0tDVmOEsy0P08HdU8L
    9+UCYiBvAK1eQlD3oGA7pvB/9DpHKLSiZOBtmss0EXuJdixKvlcF6GPHBpAjG90g
    ogwcRXJt8qJm9/N5pz+3odYFttXwBn7bdxNLBaUkG3RvrFHUslmN7V0tvFIpjAIT
    f7/5jmLhJugoP6wl9hUEsUSrcdRmSYKRNuHFU06OazBTlka4ksM3z2RFJ6TRhxXZ
    s8U8o3ECgYEAwYKeDJQcx+gRC26Vq6EWT5oHZOLrTh5QrZv/cBo0YP8nhLR0uzwz
    HNj8sMgyFV8yLCYvWaqgRCfCwMoMAUQCH5q0GPNxlQuaL+3WjcTwQeTPms9IuMFh
    rTDt1mi3xPwc5n8ZNafB8+1cNJKOCvrKXdxM/kmRIJVUaFREjyM+LgUCgYEA6cOT
    sl2fp80n10VONcFeVIEaN+YjBapDBJzaNThxTVzjBRsPyUzgEIhQ6r6V8LmG56Wo
    VfyELuvNHgKYvA6mIlsH6l3SLq+F7ohwEDVikp0yzjiMRRhhxQUsnahtHhX3JsUd
    yX2hQOLaaNfNV7gYx64a4iWizFrEa9J2wSUQuD8CgYEAmHZD9h8gCfTysPIg5EeX
    34G4/6i1wieqYw58lCNhT2bZCPpw2jBVCQ6BEPu6UhJd4mD3f4sqmGhHTkQib0DY
    93OZH+t2evrYMZkPKUWYEiKn2w4j+sUKIz1gtkRtPbtxPb237AlPi9NgiV9KoKX1
    mTwAQX1O5cAh780s8yXOUM0CgYA/zC6c+Uw/YZBEAhgsN4/lBC8Bnn9kZmlP8vbi
    m3rgoD8c/5u5Vo+4M1vSFR2ayyd0RRPCE96HZ7ddP1wrxtu0eJ+aaOyZ7TFiPj5H
    TiqO1PQur+QoX1Ufjh/1Dyhok5oWLKnKeczuhnsRLgROsmGg7XVMzvS1TPhabOAY
    KmN7xQKBgEnOjlbCT24fvolHxSJETuoq5IHjwnB/DKTMfnsFfqDPgC/rljqQMF5v
    yzPC/h0xqCh/dI7pIsJ5FjEXOtIJT/sWa1iddB7WC2oFh6AIrVJszt0dQx+4lS2m
    OgdvbViAVYsGELhg/EeJs/ig1v27BMcv2aQtZXTEHXmOd2xL93l5
    -----END RSA PRIVATE KEY-----
Frontend:
  do_not_compress_artifacts: true
  hostname: localhost
  bind_address: 0.0.0.0
  bind_port: 8100
  certificate: |
    -----BEGIN CERTIFICATE-----
    MIIDVzCCAj+gAwIBAgIQXLWPVriqs2Kut250ctZckDANBgkqhkiG9w0BAQsFADAa
    MRgwFgYDVQQKEw9WZWxvY2lyYXB0b3IgQ0EwHhcNMjIwNDEzMTcwODUyWhcNMjMw
    NDEzMTcwODUyWjA0MRUwEwYDVQQKEwxWZWxvY2lyYXB0b3IxGzAZBgNVBAMTElZl
    bG9jaXJhcHRvclNlcnZlcjCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
    AL4TlR6UFfYtUCcIkJTrfNX/7cy03L2uOBTN+w0W4x59PdJikfZuUJ4/MqCBIzho
    U/xkw3UbT5mqqomGfXWRwOXx5G2V8F3Yt9N9bubjCsJwzG2y+YSKkGG+fsNVCBYU
    kBu2Pyp42+KGZZY0ChsaRtG4wZGFgNe5GKsdRmLfAU0LrWHbUwA7TlwQR6IEKFvP
    B44qS93fFaDG4tnpaDWLBweV5wF6vuTccdSi6vjNhReZezBkUw/GLfaVu4ro4kCD
    Gi5a3LzDrMSxtJwq/9mf/hZcM9/paUxOcygGfRhvanRM4lfrbq2OD6hMOpo3P6kT
    0ff8e19c86t2SjlTj32UufcCAwEAAaN/MH0wDgYDVR0PAQH/BAQDAgWgMB0GA1Ud
    JQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAMBgNVHRMBAf8EAjAAMB8GA1UdIwQY
    MBaAFDtiEUg8KoJGbeaZF3UnLOQY6FCxMB0GA1UdEQQWMBSCElZlbG9jaXJhcHRv
    clNlcnZlcjANBgkqhkiG9w0BAQsFAAOCAQEAoyavlLn93SzAol4uVyMtr9vc3Ps8
    IMcFU4KdJLPWkNMnL8Kpp4llABFa/7MyDbuCd3fGbmkEGiOziAjpaT7NqjVCEzvO
    p7nMgs0CklGb8hfkCEuTG1GApQ5OSI1GqO0B85EnOw2n/vE/Cc9oQDXre/+ehzLi
    9FC9BG9fEFbZlCqfAPdNZszEt85/sZ3UED2sqrdrJ1LiIzYdt4VT0+zaREqdU9DT
    dtYJ3P8pwxgMl8zB6LkxGk8DovuVfDFc3RsSMq5K0btTLX/zTrdkVof6CFdYIkLq
    QEpKJHJhNhhDn7FdOW5WMaQTEptQkFYnjWgraxTdOel0YVjIZBW+IksSxw==
    -----END CERTIFICATE-----
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEAvhOVHpQV9i1QJwiQlOt81f/tzLTcva44FM37DRbjHn090mKR
    9m5Qnj8yoIEjOGhT/GTDdRtPmaqqiYZ9dZHA5fHkbZXwXdi3031u5uMKwnDMbbL5
    hIqQYb5+w1UIFhSQG7Y/Knjb4oZlljQKGxpG0bjBkYWA17kYqx1GYt8BTQutYdtT
    ADtOXBBHogQoW88HjipL3d8VoMbi2eloNYsHB5XnAXq+5Nxx1KLq+M2FF5l7MGRT
    D8Yt9pW7iujiQIMaLlrcvMOsxLG0nCr/2Z/+Flwz3+lpTE5zKAZ9GG9qdEziV+tu
    rY4PqEw6mjc/qRPR9/x7X1zzq3ZKOVOPfZS59wIDAQABAoIBAQCb3hcUgj8YJsRp
    nd5iIFG4cyygB1hUuz8F4HuUmkYYxH8jvO0Q9hlqC00KzZsCMJteh4q4x3KZApji
    OcU72hAjAB9ftersKkFkTqHY+CnklPcupetzQuVvIfV1XI9K/AXIk8Rsobs+oRNA
    /t+ipgOZCzbAjIfBXunZuCH2BgVdFWJ9uLoGNpvMbI80F94urtWJVPq25QejtkFz
    MvDidGjZ4aaTpwGqAu0eejRGObZ20DopGd9EpImLb53fCo8hn6slXPNzS3RwA80S
    wczsFlECuwcHuqUsVdhdzqHix6DI8E1o+0e33MnH1Ds4miQudshl0eq1Z9fatXKP
    9tp0BUahAoGBANj9A2tgNAKh6WvMKao/dDXBYnUVOP1vgPfff2X+oeI1WwjKbND9
    yqneHjWUaS5+emhDoGO1TxhgF1k6kVZ0rIts8WEjg+mJixUDFA09FzGLxidiQKFU
    R9KCqDdtsWdYICDIhwrEhrZAnI7i9v53SE+ACKcpPMeS22+Nwy8QOqLfAoGBAOA/
    8IOk8QCotMZR/qVx8zPtXro2gX+uRJGsuDSzcQVtqRpOHwsFlOVT+NHu2Q9OGj01
    eauV2399eGKk5s2AykYPQbaWsQfHxKvRiG9SBv93u2VRJpSzC6n5fKVK0TOb+98U
    A6PoPr2sewni4M84Wr51C5PGJvfxVBJxq9JsTiPpAoGANvRtZ0ZoJbqH+Ystijaj
    4fFmVCzZ0CXrTdvG0jgZG8dTlPhfctaz+y2MDRnXQbU6nylxd481xwCfKTQSFwlZ
    ob4nq+howj7ZgKrU6z1roFq8BsF1iOZlgkUhAVjAs2G4UVU4DlwTmhjnDbEhyPTA
    1ZGhn2RsRkdFWA1ZP5QmpZUCgYEAkxDbxzoQ5AHALJ/xhMcqXE+75BuC6h170p2X
    YNidspWsZRf+u9e5QnzDncoqiCMMij/bv2/UN9Qtc2P6CaQBA9lVm01QZG3ayWPt
    OjRtanU3bMa/qp2RdLOtzyk18cbGdBJIIOJa40GOn3kvPjcTK/zOzucQ/2JBZKcv
    rBxjUqECgYAFPLxNuQpnKtHlboMD20/Z9nApjH7gHi6sERpVLILe+2B0ddqCB7th
    M1FEMT9CrXcJ8LKniWDn+TmZOWbzhZAHxNfLeSMvRqsoP55p3iVJWgDJxGFjuxny
    mDettaeEyzL6MKCH8nYHmJr8JohfJ5CnogGFWqPo/L1KR8qtTfeYug==
    -----END RSA PRIVATE KEY-----
  dyn_dns: {}
  default_client_monitoring_artifacts:
  - Generic.Client.Stats
  GRPC_pool_max_size: 100
  GRPC_pool_max_wait: 60
  resources:
    connections_per_second: 100
    notifications_per_second: 10
    max_upload_size: 10485760
    expected_clients: 10000
Datastore:
  implementation: Test

Mail: {}
Logging:
  debug: {}
Monitoring:
  bind_address: 127.0.0.1
  bind_port: 8003
api_config: {}
defaults:
  hunt_expiry_hours: 168
  notebook_cell_timeout_min: 10
obfuscation_nonce: ""
Writeback:
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEAyUjx7T7ZCG3KL/Kln/KLo/27AYYyETRjvqfiHcTpoq11RNAd
    6j/ZtRGaI6jq7RhYGLjZ28PDWZ46np90S9BBWncq7BeAD5OWMoQ+LldKWEIeuxNf
    NEdYRAZHpleZDmrYySL8Sz/iLexPbErTsX9SgYJmq+S7zckvQpUSms9M0ERTxzGu
    Fo7e/N1wkkLozBdALrQpuGfsP0IcqqJDx+rNH+tFbkshB2CuYNsxuL7jaol0bRC2
    f0yyCBW2tUd+kTnoEEuu6QH8wIZcm76lPYDKCbYQfoUd0+Zw9RlcJbnR/wtOemFv
    4lrHIpGCyvEQsIGrbAo80dro/+W5CQ5sXVa4PQIDAQABAoIBAQCRLAYoiRJ5LM4N
    ZOEligZCwYY1pDbKB9IEuQqxU8r55EbW2Y8p2uFG4aodHABL/inTssaV5Qwov6Eh
    tHlpEIuCFr6jRpO7KEPErXI3dAePvihx3pfkmaxEa48TnswqBM/TyWLTVwDnBC+I
    ODOUKfM0qvsn8LGuyvJGAERJ2UdaURTJg4tvbB1KmVy/w0vS2FRex7+2diuNR2VU
    Wskb9Gfy1wv9Ki5wJ9oMx281pP7fr1m81QZ0fvtvHXcfpuTEPQ8gGyH9fhvV8+Wj
    r7lMK6fFBuLo/MIi7RkGHRLLk1D3AXaGGAAToMcI6VgoCLgzYrmRIYDE7lTGoTue
    lri07oLBAoGBANCQWmtM9L+uUmCWMDzRr0yI/a0erq37wFfGCZ51DzmiDUEoYMz0
    3BUq3ZQ7Uj8eYwp2KXgZRLrFcFnMZaBKwLnpLSwVtHqNkJIBoPcNCavjoDFCPENC
    9HZCLAwvJ19z2Vs4vOCIP486lljl+DLoWwqXHtUjD3zo/Og0FNL37K4xAoGBAPcQ
    xm77ilYJlorlRs2tDaeDcIJA8crCP3qY2M6fVFUsYgia072BK+jiAylJ0blVyLKo
    Vuj9fQMl5IotI/wqa1GUB2k99zqyN4RsV8+Vz33bezkEk7VM1PMX+p1ZdoOUsq0B
    4eEkQWMVpEYT07SitzLp+KiaPuCnWu4tUG7YHSvNAoGBAJDk2IxXCGnqV3yWmqiG
    HD0VpvcQq9ZfYf8YrDITrSIi/QZZYPbC3esuvoVuuPL0z2XDNYgkNeVzqVwZbjjv
    9fiykBlicuH5W4iz7Pn1atSp7O6Lz4YDDAbkbemBEN91gnmnb0CmJ1IAJ9dW3Rmw
    5x7yYg88rlPfIWTIWfc/GoThAoGACXGJtCpHMlyxdWOoHip0MCf0//WNiGt+U6hN
    +S/b4FmO8bdBSqgKTp988XIR4xylTDblA4jU427qWmG5U2UnrvmSgvJMZeD0AErH
    3HZkdPITtq03HCHwrc4H1UXbItJnNfexc5KYMTpdihQt7mSdzgNlbsRejOW4swvm
    XCZEjy0CgYAaabS1n0jBIAjCHa3ES2hukU0blDr+gUp++Lq1sHhvBLFGJZecOLV+
    +/aLDH51eP38KQCl0DLdLrSQGVOLwm+LExQWtk5nJwcqm3StYFMAhxUlFMDiBT9r
    tkaee3pnemXWJQ7R0HyIuOSqNMtg8okUoDST4RxWp2o283Iivf+AlQ==
    -----END RSA PRIVATE KEY-----
  client_id: C.1352adc54e292a23

Cloud:
  # Credentials for accessing opensearch server
  username: admin
  password: admin
  disable_ssl_security: true
  addresses:
    - http://127.0.0.1:9200/
    # Alternaive proxy endpoint for protocol inspection.
    #- http://127.0.0.1:9201/

  # S3 details for uploads.
  bucket: velociraptor
  # Endpoint for s3 emulation.
  endpoint: "https://127.0.0.1:4566/"
  # Alternaive proxy endpoint for protocol inspection.
  #endpoint: "http://localhost:9999/"
  aws_region: "us-east-1"
  credentials_key: test
  credentials_secret: test
  no_verify_cert: true
`

const writeback_file = `
private_key: |
  -----BEGIN RSA PRIVATE KEY-----
  MIIEpAIBAAKCAQEAyUjx7T7ZCG3KL/Kln/KLo/27AYYyETRjvqfiHcTpoq11RNAd
  6j/ZtRGaI6jq7RhYGLjZ28PDWZ46np90S9BBWncq7BeAD5OWMoQ+LldKWEIeuxNf
  NEdYRAZHpleZDmrYySL8Sz/iLexPbErTsX9SgYJmq+S7zckvQpUSms9M0ERTxzGu
  Fo7e/N1wkkLozBdALrQpuGfsP0IcqqJDx+rNH+tFbkshB2CuYNsxuL7jaol0bRC2
  f0yyCBW2tUd+kTnoEEuu6QH8wIZcm76lPYDKCbYQfoUd0+Zw9RlcJbnR/wtOemFv
  4lrHIpGCyvEQsIGrbAo80dro/+W5CQ5sXVa4PQIDAQABAoIBAQCRLAYoiRJ5LM4N
  ZOEligZCwYY1pDbKB9IEuQqxU8r55EbW2Y8p2uFG4aodHABL/inTssaV5Qwov6Eh
  tHlpEIuCFr6jRpO7KEPErXI3dAePvihx3pfkmaxEa48TnswqBM/TyWLTVwDnBC+I
  ODOUKfM0qvsn8LGuyvJGAERJ2UdaURTJg4tvbB1KmVy/w0vS2FRex7+2diuNR2VU
  Wskb9Gfy1wv9Ki5wJ9oMx281pP7fr1m81QZ0fvtvHXcfpuTEPQ8gGyH9fhvV8+Wj
  r7lMK6fFBuLo/MIi7RkGHRLLk1D3AXaGGAAToMcI6VgoCLgzYrmRIYDE7lTGoTue
  lri07oLBAoGBANCQWmtM9L+uUmCWMDzRr0yI/a0erq37wFfGCZ51DzmiDUEoYMz0
  3BUq3ZQ7Uj8eYwp2KXgZRLrFcFnMZaBKwLnpLSwVtHqNkJIBoPcNCavjoDFCPENC
  9HZCLAwvJ19z2Vs4vOCIP486lljl+DLoWwqXHtUjD3zo/Og0FNL37K4xAoGBAPcQ
  xm77ilYJlorlRs2tDaeDcIJA8crCP3qY2M6fVFUsYgia072BK+jiAylJ0blVyLKo
  Vuj9fQMl5IotI/wqa1GUB2k99zqyN4RsV8+Vz33bezkEk7VM1PMX+p1ZdoOUsq0B
  4eEkQWMVpEYT07SitzLp+KiaPuCnWu4tUG7YHSvNAoGBAJDk2IxXCGnqV3yWmqiG
  HD0VpvcQq9ZfYf8YrDITrSIi/QZZYPbC3esuvoVuuPL0z2XDNYgkNeVzqVwZbjjv
  9fiykBlicuH5W4iz7Pn1atSp7O6Lz4YDDAbkbemBEN91gnmnb0CmJ1IAJ9dW3Rmw
  5x7yYg88rlPfIWTIWfc/GoThAoGACXGJtCpHMlyxdWOoHip0MCf0//WNiGt+U6hN
  +S/b4FmO8bdBSqgKTp988XIR4xylTDblA4jU427qWmG5U2UnrvmSgvJMZeD0AErH
  3HZkdPITtq03HCHwrc4H1UXbItJnNfexc5KYMTpdihQt7mSdzgNlbsRejOW4swvm
  XCZEjy0CgYAaabS1n0jBIAjCHa3ES2hukU0blDr+gUp++Lq1sHhvBLFGJZecOLV+
  +/aLDH51eP38KQCl0DLdLrSQGVOLwm+LExQWtk5nJwcqm3StYFMAhxUlFMDiBT9r
  tkaee3pnemXWJQ7R0HyIuOSqNMtg8okUoDST4RxWp2o283Iivf+AlQ==
  -----END RSA PRIVATE KEY-----
client_id: C.1352adc54e292a23
`

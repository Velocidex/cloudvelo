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
    MIIDRDCCAiygAwIBAgIRAP1dus6h2AmCT/vr3RZCTlQwDQYJKoZIhvcNAQELBQAw
    GjEYMBYGA1UEChMPVmVsb2NpcmFwdG9yIENBMCAXDTIzMDQxMzE4MzI1NFoYDzIx
    MjMwMzIwMTgzMjU0WjApMRUwEwYDVQQKEwxWZWxvY2lyYXB0b3IxEDAOBgNVBAMM
    B0dSUENfR1cwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCe6djXOAP6
    vHOlt8u87heZyWMbo/k7teiCK5etclAesdkgcetEtOk91hXhf4/cQyDkH5S9KQem
    CFcYZO7/0Y8B+QpN/6pBRFKx9O/J30sxCKajUIgKrk5Y8bPsAZzz/dbg3DWIRsjS
    hvcIWOhMDvSuAQwaFc1MT+PeRVHiaD+Jk+BmaXTJVWkdaI0NEJ2uK5zvhiwzXtE7
    PbRCoCJ0nOxDe/CtJqB0ns/1+gZ1te4j+ulAVOfrzEGL1w6JP23RXAoopUgZYOn0
    t5rqAtbOrSDp0JXDPsmr5oP3eOoCCp2GGbIhlp2HthxU6ieqqoKtsnWfl6l/SSLl
    9IXUFmL1pFIVAgMBAAGjdDByMA4GA1UdDwEB/wQEAwIFoDAdBgNVHSUEFjAUBggr
    BgEFBQcDAQYIKwYBBQUHAwIwDAYDVR0TAQH/BAIwADAfBgNVHSMEGDAWgBQ7YhFI
    PCqCRm3mmRd1JyzkGOhQsTASBgNVHREECzAJggdHUlBDX0dXMA0GCSqGSIb3DQEB
    CwUAA4IBAQB3b39mSUoucO3fITDupEB93FQkReDpBnSxUN0YJxMsQJXlLDXQvZJb
    CqZ0SL0CyfDRhesvRg5BNWIG9aZ+ZJx3a7fLsdBsCNQt50JZWsC8VhjppiNqyWc6
    WXDtLBqiueVE4UOY+jnWbcDSqVZjJi7NBspAt3HwQapwjdt4TkdcA+p487lu8pvO
    yQCIcGdsA/gT/DEQZySeIuwcEpNj2kvo5G2sSyc/TDVR6Y6krhFIwTTaQT4B/E9+
    kmks/TKbaG+tCsv2YUvlRwwHKKKDIEQLhmWFHmxiHgyE5RVZq8IICOMGTCN1Ln9n
    IJyGAjGF7klkSonrtoLdER1TeBksg6sm
    -----END CERTIFICATE-----
  gw_private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEogIBAAKCAQEAnunY1zgD+rxzpbfLvO4XmcljG6P5O7XogiuXrXJQHrHZIHHr
    RLTpPdYV4X+P3EMg5B+UvSkHpghXGGTu/9GPAfkKTf+qQURSsfTvyd9LMQimo1CI
    Cq5OWPGz7AGc8/3W4Nw1iEbI0ob3CFjoTA70rgEMGhXNTE/j3kVR4mg/iZPgZml0
    yVVpHWiNDRCdriuc74YsM17ROz20QqAidJzsQ3vwrSagdJ7P9foGdbXuI/rpQFTn
    68xBi9cOiT9t0VwKKKVIGWDp9Lea6gLWzq0g6dCVwz7Jq+aD93jqAgqdhhmyIZad
    h7YcVOonqqqCrbJ1n5epf0ki5fSF1BZi9aRSFQIDAQABAoIBAAYJqH102WHbayFu
    vETvXuIu7p8MOdn07WKUuWyTnUutQiyjZ2by4LHCwo4QxKx/uG4ybPpK5sl+I6D/
    pLz/f0l55tRT1GoqaGHuhnXLEBZK19n4o1KUkNF8TXO4E/iJOnLMqxQEbHjjO9uL
    VTgekVlTHNyY23X8yxGU3KmXgGJ/tnz1DETRSi01Vz6sT4GA3zh+QX+OoASetb8Q
    HaYFWJMRMR9UnLq8ExaeCV7MJ0y9Z9uVxq2Af57+9eZWjBE7/biooxKnIRGRU68J
    HD/x2+p+ZzfvzqTy0Si8wNyd1vDIYYPbOoSt23cpEzBqB7ij4ZgXdirT6qYC4hEW
    FFWx1PkCgYEA02TPs15B5kNc3CaNIolSTZKEaaQuSxFxVuZxNs7dWaol8ebhMYWM
    3AJ0k8dNI1lKSCI/J/BF6VeSmoVNKr3Vs9JqE57SzR4RpcYx6FhcjsKxgOCLtZAN
    bsrJd7YZJQtew4/MZB5QHWMdZicQECLbKCXt/6cRtt7ztLr/TBVshR8CgYEAwHIf
    sgKqVfmONvcCVOrTER/9pf89ukNtH7JacHcs5xE848jEFzNBDwW0vg8Z0rgchQ0O
    Ugx4c88/OHT+JX01beqnY9hMV5Mp1qumpZA2Q/rhOAMVGcncJ+6YqeEnLwIZfNX6
    6xQ7asqtDdpAKkzIreo8PuQiS/UphaNA4+eabksCgYBJfBvvoG6MGxKmvQgG33Gq
    4aoCBz7IfbHGoajtgo/T4Z/7LWVPD7vdp0TbMkcQaLO3y5/kxFOpP/YInRosJ32o
    Wxbg5y8kerVryTAEMuNKBUgrIuOuI/tnbjsG0FiBViiFFvHYQ+lZreDEaAPfeB5z
    IGxRmMRBq9NQGkkxK6ljxQKBgFE+3Qq1/VuWo+eomJ9pE/qi2t79xv2gAa3kCjJ4
    3cgfiulPlRmGVe0Vp5ylm21OtRumy2jwQtoBoNsg6TrChZAGBO0uH+zJAFzU0uIK
    5B4HCJYxFvNwOTXSkTkHCRfbdw8w92HPhNYtAqpafcRd7kseHJkgjyoqMoFszrRo
    ztXJAoGAaovW+JrgG+7xhVE2Ha1cMBhGcZ22RXM6/U7B7UgLF7jE0zZ0Z83sv6NS
    W5gJrneEd85yOMZRQ+zR9lUBnfK+csnfdPys6Tf+lnTBlXGtXaN69rmAHD1WJp5l
    JL5WubPDAGJoNCt7TqNBOwMk6avXZPFQkVljQclVoysIBQ44Tac=
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
    MIIDWTCCAkGgAwIBAgIQcyUFy1oMUr4O4sIOhom/jDANBgkqhkiG9w0BAQsFADAa
    MRgwFgYDVQQKEw9WZWxvY2lyYXB0b3IgQ0EwIBcNMjMwNDEzMTgzMjUzWhgPMjEy
    MzAzMjAxODMyNTNaMDQxFTATBgNVBAoTDFZlbG9jaXJhcHRvcjEbMBkGA1UEAxMS
    VmVsb2NpcmFwdG9yU2VydmVyMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
    AQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1Qnfn1JO3Xo6p
    53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bBoPNSrMF7+2/t
    DvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOuEuPQmR3HyHfs
    dsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr03mUu07TOUbRp
    X3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw1D7WBoElECj9
    +89CRXlu3K15P1cNVB5htPzOgwIDAQABo38wfTAOBgNVHQ8BAf8EBAMCBaAwHQYD
    VR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYDVR0j
    BBgwFoAUO2IRSDwqgkZt5pkXdScs5BjoULEwHQYDVR0RBBYwFIISVmVsb2NpcmFw
    dG9yU2VydmVyMA0GCSqGSIb3DQEBCwUAA4IBAQAhwcTMIdHqeR3FXOUREGjkjzC9
    vz+hPdXB6w9CMYDOAsmQojuo09h84xt7jD0iqs/K1WJpLSNV3FG5C0TQXa3PD1l3
    SsD5p4FfuqFACbPkm/oy+NA7E/0BZazC7iaZYjQw7a8FUx/P+eKo1S7z7Iq8HfmJ
    yus5NlnoLmqb/3nZ7DyRWSo9HApmMdNjB6oJWrupSJajsw4Lsos2aJjkfzkg82W7
    aGSh9S6Icn1f78BAjJVLv1QBNlb+yGOhrcUWQHERPEpkb1oZJwkVVE1XCZ1C4tVj
    PtlBbpcpPHB/R5elxfo+We6vmC8+8XBlNPFFp8LAAile4uQPVQjqy7k/MZ4W
    -----END CERTIFICATE-----
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1
    Qnfn1JO3Xo6p53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bB
    oPNSrMF7+2/tDvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOu
    EuPQmR3HyHfsdsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr0
    3mUu07TOUbRpX3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw
    1D7WBoElECj9+89CRXlu3K15P1cNVB5htPzOgwIDAQABAoIBAGAAy3gLOZ6hBgpU
    FR7t3C2fRAFrogxozfHRw9Xc69ZIE67lXdGxSAvX2F9NI5T09c4Stt1HLoCYHH6B
    Igbjc3XiNwI/0XY7L37PgItrLI2Q0vXUw3OGnJHH3gIz10472cPsQbuvrCi9Zu6K
    ElijnewNCM8Sx+AZCWE1zO4P9+Z2kF9LvWzDwAa643jQ/Dg+S68zCFqjJCVJBGm+
    LQxDs6dbArvOiEbuZs2wDt0d1kZF+BRljUTMoCpdf3jmFj3f0Jc1AFaz1eHG9Gte
    XIUpbWmV2ATABSW2kDkVdXx+m/w1r9PZCLLfq54fIOlm2IeAiM3rDmM4ZSTUYEPn
    mJP03xECgYEA+jS7DiS3bB/MeD+5qsgS07qJhOrX17s/SlamC1dQqz+koJLl98JX
    CqyafFmdSz7PK2S2+OOazngwx26Kc3MZFoD9IQ2tuWmwDgbY8EQs5Cs37By2YRZJ
    DdjvVf48pCKiXxIhvFjW/5CTemNAAu4CXg5Lkp7UVVrOmf5BmjMmE0sCgYEA+m+U
    QMF0f7KLM4MU81yAMJdG4Sq4s9i4RmXes2FOUd4UoG7vEpycMKkmEaqiUVmRHPjp
    P6Dwq3CK+FVFMpCeWjn6KkxwpdWWO9lglI0npFcPNW/PzPOv4mSNtCAcpHrKFP0R
    3jbc8UhgtFxDZoeUih7cO2iTO7kELBCeKUzw9qkCgYBgVYcj1e0tWzztm5OP9sKQ
    9MRYAdei/zxKEfySZ0bu+G0ZShXzA8dhm71LXXGbdA5t5bQxNej3z/zv/FagRtOE
    /5r2a/7UYaXgcLB8KbOjEiTQ6ukpjlwIUdssn9uXUqJzulZ03zvAYFj4CVivCBav
    Qg/E3xRf3LupPOTjSwhA6wKBgQDAH3tnlkHueSWLNiOLc0owfM12jhS2fCsabqpD
    iQHRkoLWdWRZLeYw+oLnCLWPnRvTUy11j90yWJt0Wc5FNWcWJuZBLvU4c7vWXDRY
    olVoIRXc09NiEwy6rJN9PSlcEYsYQPFFPWeQfwsZMrLOZHLS50vjE53oMk7+Ex2S
    56DwSQKBgQC+iHbsbxloZjVMy01V21Sh9RwIpYrodEmwlTZf2jzaYloPadHu4MX1
    jHG+zzeC/EJ3wFOKTSJ/Tmjo6N3Xaq9V7WeL8eBdtBtPztqN1yveTt94mZZ+fuID
    BhI8P2RbNR2Yey5nnhFQcoTxpmVw3EYwE01nkxoPJRs/QVvxi9Mepg==
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
  endpoint: "http://127.0.0.1:4566/"
  # Alternaive proxy endpoint for protocol inspection.
  #endpoint: "http://localhost:9999/"
  aws_region: "us-east-1"
  credentials_key: admin
  credentials_secret: password
  no_verify_cert: true
  foreman_interval_seconds: 5
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

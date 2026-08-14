import socket
import sys
import threading


def recv_exact(conn, size):
    data = b""
    while len(data) < size:
        chunk = conn.recv(size - len(data))
        if not chunk:
            raise ConnectionError("closed")
        data += chunk
    return data


def handle(conn):
    try:
        recv_exact(conn, 3)
        conn.sendall(b"\x05\x00")
        header = recv_exact(conn, 4)
        address_type = header[3]
        if address_type == 1:
            recv_exact(conn, 4)
        elif address_type == 4:
            recv_exact(conn, 16)
        elif address_type == 3:
            recv_exact(conn, recv_exact(conn, 1)[0])
        recv_exact(conn, 2)
        conn.sendall(b"\x05\x00\x00\x01\x00\x00\x00\x00\x00\x00")
        conn.recv(4096)
        with open(sys.argv[2], "a", encoding="utf-8") as log:
            log.write("hit\n")
        conn.sendall(
            b"HTTP/1.1 200 OK\r\n"
            b"Content-Length: 7\r\n"
            b"Connection: close\r\n"
            b"\r\n"
            b"mock-ok"
        )
    except Exception:
        pass
    finally:
        conn.close()


def main():
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(("127.0.0.1", int(sys.argv[1])))
    server.listen()
    while True:
        conn, _ = server.accept()
        threading.Thread(target=handle, args=(conn,), daemon=True).start()


if __name__ == "__main__":
    main()

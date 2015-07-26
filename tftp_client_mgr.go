package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

//TFTPClientMgr manages TFTP clients
type TFTPClientMgr struct {
	Connections map[string]*TFTPConn
	wg          sync.WaitGroup
}

//Start starta new TFTP client session
func (c *TFTPClientMgr) Start(addr *net.UDPAddr, msg interface{}) {
	//add connection
	if tftpMsg, ok := msg.(*TFTPReadWritePkt); ok {
		nc := &TFTPConn{Type: tftpMsg.Opcode, remote: addr, blockSize: DefaultBlockSize, filename: msg.(*TFTPReadWritePkt).Filename}
		c.Connections[addr.String()] = nc
		if tftpMsg.Opcode == OpcodeRead {
			//Setting block to min of 1
			c.Connections[addr.String()].block = 1
			c.sendData(addr.String())
		} else if tftpMsg.Opcode == OpcodeWrite {
			//Setting block to min of 0
			c.Connections[addr.String()].block = 0
			c.recieveData(addr.String())
		}
	}
	//send error message
}

//ACK handle ack packet
func (c *TFTPClientMgr) ACK(addr *net.UDPAddr, msg interface{}) {
	if tftpMsg, ok := msg.(*TFTPAckPkt); ok {
		log.Printf("%#v", tftpMsg)
	}
}

func (c *TFTPClientMgr) sendAck(conn *net.UDPConn, tid string) {
	pkt := &TFTPAckPkt{Opcode: OpcodeACK, Block: c.Connections[tid].block}
	conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	if _, err := conn.Write(pkt.Pack()); err != nil {
		log.Println(err)
	}
}

func (c *TFTPClientMgr) sendError(opcode int, tid string) {

}

func (c *TFTPClientMgr) sendData(tid string) {
	//TODO: Implement reverse of recieve data
	//read from file send to destination, update block
	if r, err := net.DialUDP(ServerNet, nil, c.Connections[tid].remote); err != nil {
		log.Println(err)
	} else {
		buffer := make([]byte, c.Connections[tid].blockSize)
		inputFile, err := os.OpenFile(c.Connections[tid].filename, os.O_RDWR|os.O_CREATE, 0660)
		defer inputFile.Close()
		if err != nil {
			//Unable to open file, send error to client
			log.Println(err)
		}
		inputReader := bufio.NewReader(inputFile)
		for {
			dLen, err := inputReader.Read(buffer)
			log.Println(c.Connections[tid].blockSize, dLen)
			if err != nil {
				//unable to read from file
				log.Println(err)
			}
			pkt := &TFTPDataPkt{Opcode: OpcodeData, Block: c.Connections[tid].block, Data: buffer}
			r.SetWriteDeadline(time.Now().Add(1 * time.Second))
			if _, err := r.Write(pkt.Pack()); err != nil {
				log.Println(err)
			}
			buffer = buffer[:cap(buffer)]
			//TODO: send next packet once block is sent
			c.Connections[tid].block = c.Connections[tid].block + 1
			if c.Connections[tid].blockSize > dLen {
				return
			}
		}
	}
}

func (c *TFTPClientMgr) recieveData(tid string) {
	if r, err := net.DialUDP(ServerNet, nil, c.Connections[tid].remote); err != nil {
		log.Println(err)
	} else {
		c.sendAck(r, tid)
		bb := make([]byte, 1024000)
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			outputFile, err := os.OpenFile(c.Connections[tid].filename, os.O_RDWR|os.O_CREATE, 0660)
			if err != nil {
				//Unable to open file, send error to client
				log.Println(err)
			}
			for {
				//handle each packet in a seperate go routine
				msgLen, _, _, _, err := r.ReadMsgUDP(bb, nil)
				if err != nil {
					switch err := err.(type) {
					case net.Error:
						if err.Timeout() {
							log.Println(err)
						} else if err.Temporary() {
							log.Println(err)
						}
					}
					return
				}
				//pull message from buffer
				msg := bb[:msgLen]
				//clear buffer
				bb = bb[:cap(bb)]
				if uint16(msg[1]) == OpcodeData {
					pkt := &TFTPDataPkt{}
					pkt.Unpack(msg)
					//	log.Printf("%#v", pkt)
					//Write Data
					ofb, err := outputFile.Write(pkt.Data)
					if err != nil {
						//Unable to write to file
						log.Println(err)
					}
					log.Printf("Wrote %d bytes to file %s", ofb, c.Connections[tid].filename)
					c.Connections[tid].block = pkt.Block
					if len(pkt.Data) < DefaultBlockSize {
						//last packet
						c.sendAck(r, tid)
						err := r.Close()
						if err != nil {
							panic(err)
						}
						return
					}
					//continue to read data
					c.sendAck(r, tid)
					//TODO: Write data
				} else {
					//TODO: send error
				}
			}
		}()

	}
}

//TFTPConn TFTP connection
type TFTPConn struct {
	Type      uint16
	remote    *net.UDPAddr
	block     uint16
	blockSize int
	filename  string
}

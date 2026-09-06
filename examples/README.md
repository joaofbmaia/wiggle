# Examples

Each diagram rendered with `wiggle -p`, captured by [`docs/shot.sh`](../docs/shot.sh). The [tutorial](tutorial/) folder has every step of the WaveDrom tutorial.

## axi-handshake

AXI-style valid/ready handshake with back-pressure and a wait state.

![axi-handshake](axi-handshake.svg)

## ddr

DDR read: command on the rising edge, data on both edges of DQS.

![ddr](ddr.svg)

## edges

The WaveDrom edge tutorial: every arrow style.

![edges](edges.svg)

## gaps

Time breaks: a long transaction with the boring middle cut out.

![gaps](gaps.svg)

## i2c

I2C start, address byte, ACK, one data byte, stop.

![i2c](i2c.svg)

## pipeline

Classic 5-stage pipeline with a load-use stall.

![pipeline](pipeline.svg)

## showcase

A little of everything: the README picture.

![showcase](showcase.svg)

## spi

SPI mode 0: MOSI/MISO sampled on the rising edge of SCLK.

![spi](spi.svg)

## sram

Synchronous SRAM: two writes then two reads, one cycle latency.

![sram](sram.svg)

## states

Every wave character in one place.

![states](states.svg)

## uart

8N1 UART frame: start bit, LSB-first data, stop bit.

![uart](uart.svg)

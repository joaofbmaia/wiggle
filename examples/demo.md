# SPI transfer

A **mode 0** transaction: data is sampled on the rising edge of `sclk`.

```wavedrom
{ signal: [
  { name: 'cs_n',  wave: '10.......1' },
  { name: 'sclk',  wave: '0.P......0' },
  { name: 'mosi',  wave: 'x.=.=.=.=x', data: ['b7', 'b6', 'b5', 'b4'] },
  { name: 'miso',  wave: 'z.=.=.=.=z', data: ['r7', 'r6', 'r5', 'r4'] },
]}
```

Some regular code stays as it is:

```go
fmt.Println("hello")
```

And a broken block reports its error instead of failing the document:

```wavedrom
{ signal: [ { name: oops } ] }
```

- lists still work
- and so on
